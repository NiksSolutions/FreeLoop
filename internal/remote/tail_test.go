package remote

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// Issue 046: the phone opens a session at its end. With tail the stream starts at the
// session's last events, then goes on live; without it, from the start as before.
func TestPhoneStreamOpensAtTheEnd(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	ctx := context.Background()
	ses, err := e.CreateSession(ctx, engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := e.AppendEvent(ctx, engine.AppendEventRequest{SessionID: ses.ID, Type: event.Notice, Data: []byte(`{"message":"x"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := e.Events(ctx, ses.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := all[len(all)-1].Seq
	firstID := func(query string) int64 {
		req, _ := http.NewRequest("GET", p.base+"/api/sessions/"+ses.ID+"/stream?"+query, nil)
		p.authorize(req)
		resp, err := p.client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("stream %s: %v %v", query, resp.StatusCode, err)
		}
		defer resp.Body.Close()
		lines := bufio.NewScanner(resp.Body)
		for lines.Scan() {
			if id, ok := strings.CutPrefix(lines.Text(), "id: "); ok {
				n, _ := strconv.ParseInt(id, 10, 64)
				return n
			}
		}
		t.Fatalf("stream %s sent no event", query)
		return 0
	}
	if got := firstID("after=0&tail=3"); got != last-2 {
		t.Errorf("with tail=3 the stream starts at seq %d, want %d", got, last-2)
	}
	if got := firstID("after=0"); got != 1 {
		t.Errorf("without tail the stream starts at seq %d, want 1", got)
	}
	if got := firstID("after=5&tail=3"); got != 6 {
		t.Errorf("taken up again after 5, the stream starts at seq %d", got)
	}
}

// dropping is the engine's bus losing the events drop picks, as it does for a subscriber
// that falls behind.
func dropping(e *engine.Engine, drop func(event.Event) bool) func() (<-chan event.Event, func()) {
	return func() (<-chan event.Event, func()) {
		in, cancel := e.Subscribe()
		out := make(chan event.Event, 64)
		go func() {
			defer close(out)
			for ev := range in {
				if !drop(ev) {
					select {
					case out <- ev:
					default:
					}
				}
			}
		}()
		return out, cancel
	}
}

// lines reads a stream's lines as they come.
func lines(t *testing.T, p *phone, path string) <-chan string {
	t.Helper()
	req, _ := http.NewRequest("GET", p.base+path, nil)
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream %s: %v", path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	out := make(chan string, 256)
	go func() {
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			out <- sc.Text()
		}
	}()
	return out
}

// waitLine waits up to three seconds for a line ok picks.
func waitLine(t *testing.T, ch <-chan string, what string, ok func(string) bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case l, open := <-ch:
			if !open {
				t.Fatalf("the stream ended before %s", what)
			}
			if ok(l) {
				return
			}
		case <-deadline:
			t.Fatalf("no %s within a keep-alive tick", what)
		}
	}
}

// 076: an event the bus dropped, with none after it to show the gap, still reaches the
// phone: the keep-alive tick catches up. Before, a dropped turn_ended left the phone
// showing a working session.
func TestADroppedLastEventArrivesWithinATick(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	s.keepAlive = 100 * time.Millisecond
	ctx := context.Background()
	ses, err := e.CreateSession(ctx, engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	s.subscribe = dropping(e, func(ev event.Event) bool { return ev.SID == ses.ID })
	stream := lines(t, p, "/api/sessions/"+ses.ID+"/stream?after=0")
	waitLine(t, stream, "the session's first event", func(l string) bool { return strings.HasPrefix(l, "id: ") })
	ev, err := e.AppendEvent(ctx, engine.AppendEventRequest{SessionID: ses.ID, Type: event.Notice, Data: []byte(`{"message":"the last one"}`)})
	if err != nil {
		t.Fatal(err)
	}
	waitLine(t, stream, "dropped event", func(l string) bool { return l == "id: "+strconv.FormatInt(ev.Seq, 10) })

	// The lists' stream likewise: a new session shows though the bus said nothing.
	s.subscribe = dropping(e, func(event.Event) bool { return true })
	sum := lines(t, p, "/api/stream")
	waitLine(t, sum, "first summary", func(l string) bool { return strings.HasPrefix(l, "data: ") })
	other, err := e.CreateSession(ctx, engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted", Title: "Caught up"})
	if err != nil {
		t.Fatal(err)
	}
	waitLine(t, sum, "summary with the new session", func(l string) bool { return strings.HasPrefix(l, "data: ") && strings.Contains(l, other.ID) })
}
