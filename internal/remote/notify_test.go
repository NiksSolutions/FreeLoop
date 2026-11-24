package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
)

// pushOf waits for one push to the fake service and reads what it carries.
func pushOf(t *testing.T, f *pushed, read func([]byte) []byte) map[string]any {
	t.Helper()
	select {
	case <-f.got:
	case <-time.After(10 * time.Second):
		t.Fatal("no push arrived")
	}
	var out map[string]any
	if err := json.Unmarshal(read(<-f.body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func noPush(t *testing.T, f *pushed, why string) {
	t.Helper()
	select {
	case <-f.got:
		t.Errorf("a push went out: %s", why)
	case <-time.After(500 * time.Millisecond):
	}
}

func notifiedSetup(t *testing.T) (*engine.Engine, *Server, *phone, *pushed, func([]byte) []byte) {
	t.Helper()
	e, s, p := started(t)
	f := fakePush(t, s, http.StatusCreated)
	_, ua, auth := subscribed(t, s, p, "https://fcm.googleapis.com/fcm/send/abc")
	return e, s, p, f, func(b []byte) []byte {
		plain, err := decryptPush(b, ua, auth)
		if err != nil {
			t.Fatal(err)
		}
		return plain
	}
}

// 029: an approval in a demo session pushes once to a phone that turned it on and
// holds no stream, and carries the kind and the session and nothing else.
func TestAnApprovalPushesOnce(t *testing.T) {
	e, _, _, f, read := notifiedSetup(t)
	ses, err := e.StartDemo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := pushOf(t, f, read)
	if len(got) != 2 || got["kind"] != "approval" || got["sessionId"] != ses.ID {
		t.Errorf("payload: %v", got)
	}
	noPush(t, f, "a second one for the same request")
}

// 029: a phone with a stream open is looking at the sessions, and is not pushed to.
func TestNoPushToAPhoneWithAStreamOpen(t *testing.T) {
	e, _, p, f, _ := notifiedSetup(t)
	req, _ := http.NewRequest("GET", p.base+"/api/stream", nil)
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v", err)
	}
	defer resp.Body.Close()
	bufio.NewReader(resp.Body).ReadString('\n') // it is registered once it answers
	if _, err := e.StartDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	noPush(t, f, "to a phone whose stream is open")
}

// 029: notifications turned off push nothing.
func TestNoPushWhenNotificationsAreOff(t *testing.T) {
	e, _, _, f, _ := notifiedSetup(t)
	cfg := e.Config()
	cfg.Notifications.Enabled = false
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	noPush(t, f, "with notifications off")
}

// 029: a muted project pushes nothing: the first request pushes, then the project is
// muted and the request answered, and the next does not.
func TestNoPushForAMutedProject(t *testing.T) {
	e, _, _, f, read := notifiedSetup(t)
	ses, err := e.StartDemo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pushOf(t, f, read)
	if err := e.SetProjectMuted(context.Background(), ses.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.PendingPermissions() {
		e.Decide(engine.DecideRequest{RequestID: r.ID, OptionID: "allow_once"})
	}
	noPush(t, f, "for a muted project")
}

// 029: a notice's kind becomes the kind the worker has words for, and nothing is
// pushed in local-only mode.
func TestNoticeKindsAndLocalOnly(t *testing.T) {
	for in, want := range map[string]string{"approval": "approval", "finished": "done", "error": "failed", "budget": "budget", "x": "other"} {
		if got := pushKind(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
	e, s, _, f, _ := notifiedSetup(t)
	cfg := e.Config()
	cfg.Network.LocalOnly = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	s.Notify(engine.Notice{Kind: "finished", SessionID: "ses_1"})
	noPush(t, f, "in local-only mode")
}

// waitUntil polls ok for up to three seconds.
func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); !ok(); {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// lookingAt is how many streams a phone has open.
func lookingAt(s *Server, id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.looking[id]
}

// 076: only an open stream says the phone is being looked at. Any other call in flight,
// a slow message say, does not hold the push back, and a stream closed, as the phone app
// does when it is hidden, lets it through.
func TestOnlyAnOpenStreamIsLooking(t *testing.T) {
	_, s, p, f, read := notifiedSetup(t)
	id := pairedID(s)
	// A call that is not a stream, held open.
	release := make(chan struct{})
	entered := make(chan struct{})
	slow := s.authed(func(w http.ResponseWriter, r *http.Request, _ config.Device) {
		close(entered)
		<-release
	})
	req := httptest.NewRequest("GET", "/api/summary", nil)
	p.authorize(req)
	done := make(chan struct{})
	go func() {
		slow(httptest.NewRecorder(), req)
		close(done)
	}()
	<-entered
	s.Notify(engine.Notice{Kind: "approval", SessionID: "ses_1"})
	if got := pushOf(t, f, read); got["sessionId"] != "ses_1" {
		t.Errorf("payload: %v", got)
	}
	close(release)
	<-done

	// A stream open: no push; closed: the next one goes.
	sreq, _ := http.NewRequest("GET", p.base+"/api/stream", nil)
	p.authorize(sreq)
	resp, err := p.client.Do(sreq)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v", err)
	}
	bufio.NewReader(resp.Body).ReadString('\n')
	waitUntil(t, "the stream to count", func() bool { return lookingAt(s, id) == 1 })
	s.Notify(engine.Notice{Kind: "approval", SessionID: "ses_2"})
	noPush(t, f, "to a phone whose stream is open")
	resp.Body.Close()
	waitUntil(t, "the closed stream to stop counting", func() bool { return lookingAt(s, id) == 0 })
	s.Notify(engine.Notice{Kind: "finished", SessionID: "ses_3"})
	if got := pushOf(t, f, read); got["kind"] != "done" || got["sessionId"] != "ses_3" {
		t.Errorf("payload after the stream closed: %v", got)
	}
}

// stalled is a listener whose connections, once stall is set, take no more writes: as a
// phone gone out of reach with its connection held open, whose buffers are full. A
// write waits for its deadline, if it has one.
type stalled struct {
	net.Listener
	stall atomic.Bool
}

func (l *stalled) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &stallConn{Conn: c, l: l, closed: make(chan struct{})}, nil
}

type stallConn struct {
	net.Conn
	l        *stalled
	mu       sync.Mutex
	deadline time.Time
	closed   chan struct{}
	once     sync.Once
}

func (c *stallConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}

func (c *stallConn) Write(b []byte) (int, error) {
	if !c.l.stall.Load() {
		return c.Conn.Write(b)
	}
	c.mu.Lock()
	wait := time.Until(c.deadline)
	if c.deadline.IsZero() {
		wait = time.Minute
	}
	c.mu.Unlock()
	select {
	case <-time.After(wait):
	case <-c.closed:
	}
	return 0, os.ErrDeadlineExceeded
}

func (c *stallConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// 076: a stream whose phone stopped taking what is written ends within the write's
// deadline, and the phone no longer counts as looking.
func TestAStalledStreamEnds(t *testing.T) {
	_, s, p := started(t)
	pairPhone(t, s, p)
	id := pairedID(s)
	s.keepAlive, s.writeWait = 50*time.Millisecond, 200*time.Millisecond
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &stalled{Listener: ln}
	srv := &http.Server{Handler: s.Handler()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	req, _ := http.NewRequest("GET", "http://"+ln.Addr().String()+"/api/stream", nil)
	req.Host = "laptop.tail1234.ts.net"
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v", err)
	}
	defer resp.Body.Close()
	bufio.NewReader(resp.Body).ReadString('\n')
	waitUntil(t, "the stream to count", func() bool { return lookingAt(s, id) == 1 })
	l.stall.Store(true)
	waitUntil(t, "the stalled stream to end", func() bool { return lookingAt(s, id) == 0 })
}
