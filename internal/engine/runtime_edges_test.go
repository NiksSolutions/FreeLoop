package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// answeringSession is a session on a scripted provider that answers "done" to each
// prompt.
func answeringSession(t *testing.T, e *Engine) string {
	t.Helper()
	steps := make([]scripted.Step, 10)
	for i := range steps {
		steps[i] = scripted.Step{Text: "done"}
	}
	withScript(e, "s", steps...)
	ses, err := e.CreateSession(context.Background(), CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "s", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	return ses.ID
}

func typesOf(evs []event.Event) []string {
	out := []string{}
	for _, ev := range evs {
		out = append(out, string(ev.Type))
	}
	return out
}

// waitNotBusy waits for a session's runtime to let go of its last turn.
func waitNotBusy(t *testing.T, e *Engine, sid string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		e.mu.Lock()
		r := e.runtimes[sid]
		e.mu.Unlock()
		busy := false
		if r != nil {
			r.mu.Lock()
			busy = r.busy
			r.mu.Unlock()
		}
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the runtime stays busy")
		}
	}
}

// SES-014, issue 080: the checkpoint before a turn runs under the turn's context. The
// session shows it is running while the snapshot is taken, Stop during it ends the turn
// at once with the message recorded and its end cancelled, and what the cut-off git
// left in the store does not turn the next turn's checkpoint off.
func TestStopDuringTheCheckpointEndsTheTurnAtOnce(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := answeringSession(t, e)
	in := make(chan struct{})
	first := true
	snapshotHook = func(ctx context.Context, store string) {
		if !first {
			return
		}
		first = false
		// What a git add killed in the middle leaves behind.
		os.WriteFile(filepath.Join(store, "index.lock"), nil, 0o600)
		close(in)
		<-ctx.Done()
	}
	t.Cleanup(func() { snapshotHook = nil })

	if _, err := e.Prompt(ctx, sid, "first"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-in:
	case <-time.After(10 * time.Second):
		t.Fatal("no snapshot started")
	}
	if ses, _ := e.store.GetSession(ctx, sid); ses.Status != "running" {
		t.Errorf("status while the snapshot is taken = %s", ses.Status)
	}
	start := time.Now()
	e.Cancel(sid)
	end := waitFor(t, ch, func(ev event.Event) bool { return ev.SID == sid && ev.Type == event.TurnEnded })
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the stop took %v", took)
	}
	var d event.TurnEndedData
	if json.Unmarshal(end.Data, &d); d.StopReason != "cancelled" {
		t.Errorf("stop reason = %q", d.StopReason)
	}
	waitNotBusy(t, e, sid)
	evs, _ := e.Events(ctx, sid, 0, 0)
	got := strings.Join(typesOf(evs), ",")
	// The message is kept, the stop said after it, and the turn and status end.
	if !strings.HasSuffix(got, "session_status,turn_started,user_message_chunk,notice,turn_ended,session_status") {
		t.Errorf("events = %s", got)
	}
	if slices.Contains(typesOf(evs), string(event.Checkpoint)) || !hasNotice(t, e, sid, "You stopped the turn.") {
		t.Errorf("a checkpoint was recorded, or the stop not said: %s", got)
	}
	if ses, _ := e.store.GetSession(ctx, sid); ses.Status != "idle" {
		t.Errorf("status after the stop = %s", ses.Status)
	}

	if _, err := e.Prompt(ctx, sid, "second"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, sid)
	evs, _ = e.Events(ctx, sid, 0, 0)
	if !slices.Contains(typesOf(evs), string(event.Checkpoint)) || hasNotice(t, e, sid, "Checkpoints are off") {
		t.Errorf("the next turn's checkpoint failed: %s", strings.Join(typesOf(evs), ","))
	}
}

// NFR-REL-002, issue 080: the runtime of an agent that exited is stopped before the
// session hears of it, and stopping it later leaves alone a runtime started since.
func TestAnExitedAgentsRuntimeIsStoppedAtOnce(t *testing.T) {
	ctx := context.Background()
	e, ses := referenceAgentSession(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	e.Prompt(ctx, ses.ID, "one")
	stopEarlyIn(t, e, ch, ses.ID)
	waitNotBusy(t, e, ses.ID)
	e.mu.Lock()
	first := e.runtimes[ses.ID]
	e.mu.Unlock()
	first.hs.Close()
	if _, err := e.Prompt(ctx, ses.ID, "two"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.SID == ses.ID && ev.Type == event.Notice && strings.Contains(string(ev.Data), "starts it again")
	})
	first.mu.Lock()
	stopped := first.stopped
	first.mu.Unlock()
	if !stopped {
		t.Error("the runtime of the exited agent can still take a prompt")
	}

	// A runtime started while the old one waited to be stopped stays.
	old := &runtime{sid: "x", host: &environment.Local{}}
	fresh := putIdleRuntime(e, "x")
	e.retire(old)
	if !hasRuntime(e, "x") || fresh.stopped || !old.stopped {
		t.Errorf("retiring the old runtime: fresh kept %v, fresh stopped %v, old stopped %v", hasRuntime(e, "x"), fresh.stopped, old.stopped)
	}
}

// OQ-053, issue 080: a prompt that meets a runtime the idle stop is stopping starts a
// new one, since a scheduled, AFK or phone prompt has nobody to send it again; while
// the app quits it does not, and the session's status stays as it was.
func TestAPromptThatMeetsTheIdleStopStartsANewRuntime(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	sid := answeringSession(t, e)
	stale := putIdleRuntime(e, sid)
	stale.stopped, stale.replace = true, true
	if _, err := e.Prompt(ctx, sid, "hello"); err != nil {
		t.Fatalf("a prompt that met the idle stop: %v", err)
	}
	waitIdle(t, ch, sid)
	e.mu.Lock()
	now := e.runtimes[sid]
	e.mu.Unlock()
	if now == nil || now == stale {
		t.Error("no new runtime took the prompt")
	}

	// While the app quits, nothing starts and nothing is recorded.
	other := answeringSession(t, e)
	gone := putIdleRuntime(e, other)
	gone.stopped, gone.replace = true, true
	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	if _, err := e.Prompt(ctx, other, "hello"); !errors.Is(err, errClosing) {
		t.Errorf("a prompt while the app quits = %v", err)
	}
	evs, _ := e.Events(ctx, other, 0, 0)
	if ses, _ := e.store.GetSession(ctx, other); ses.Status == "error" || slices.Contains(typesOf(evs), string(event.Error)) {
		t.Errorf("a start refused at quit was recorded as an error: %s, %v", ses.Status, typesOf(evs))
	}
}

// Issue 080: a start that fails because the app quits leaves the session's status
// as it was.
func TestAStartRefusedAtQuitLeavesTheStatus(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	sid := answeringSession(t, e)
	e.mu.Lock()
	e.closing = true
	e.mu.Unlock()
	if _, err := e.Prompt(ctx, sid, "hello"); !errors.Is(err, errClosing) {
		t.Fatalf("prompt = %v", err)
	}
	evs, _ := e.Events(ctx, sid, 0, 0)
	if ses, _ := e.store.GetSession(ctx, sid); ses.Status != "idle" || slices.Contains(typesOf(evs), string(event.Error)) {
		t.Errorf("status %s, events %v", ses.Status, typesOf(evs))
	}
	if hasRuntime(e, sid) {
		t.Error("a runtime started while the app quits")
	}
}

// GUI-004, issue 080: queued messages that are dropped are said in the session, how
// many and why: by Stop, by quitting, and by a stop of the runtime.
func TestDroppedQueuedMessagesAreSaid(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	ses, _ := demoSession(t, e, "demo", "ask")
	e.Prompt(ctx, ses.ID, "first")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	for _, text := range []string{"second", "third"} {
		if res, err := e.Prompt(ctx, ses.ID, text); err != nil || !res.Queued {
			t.Fatalf("%s = %+v, %v", text, res, err)
		}
	}
	e.Cancel(ses.ID)
	if !hasNotice(t, e, ses.ID, "You stopped the turn. 2 queued messages were not sent.") {
		t.Error("Stop did not say the queued messages were dropped")
	}
	waitNotBusy(t, e, ses.ID)

	// A stop of the runtime itself, as when a subagent's work ends.
	e.stopRuntime(ses.ID)
	r := putIdleRuntime(e, ses.ID)
	r.queue = []queued{{text: "later"}}
	e.stopRuntime(ses.ID)
	if !hasNotice(t, e, ses.ID, "1 queued message was not sent") {
		t.Error("stopping the runtime did not say the queued message was dropped")
	}

	// Quitting.
	e.Prompt(ctx, ses.ID, "again")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested })
	e.Prompt(ctx, ses.ID, "queued at quit")
	unsub()
	e.Close()
	e = openTest(t, dir)
	defer e.Close()
	if !hasNotice(t, e, ses.ID, "Stopped because Freeloop quit. 1 queued message was not sent.") {
		t.Error("quitting did not say the queued message was dropped")
	}
}
