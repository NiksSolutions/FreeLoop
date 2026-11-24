package engine

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// crash leaves an engine as a power loss would: nothing is stopped or closed in order.
func crash(e *Engine) {
	e.mu.Lock()
	e.runtimes = map[string]*runtime{}
	e.mu.Unlock()
	e.bus.close()
	e.store.Close()
}

// lastTypes are the types of a session's last n events.
func lastTypes(t *testing.T, e *Engine, sid string, n int) string {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0, 0)
	if err != nil || len(evs) < n {
		t.Fatalf("events = %d, %v", len(evs), err)
	}
	var types []string
	for _, ev := range evs[len(evs)-n:] {
		types = append(types, string(ev.Type))
	}
	return strings.Join(types, ",")
}

func lastSeq(t *testing.T, e *Engine, sid string) int64 {
	t.Helper()
	evs, err := e.Events(context.Background(), sid, 0, 0)
	if err != nil || len(evs) == 0 {
		t.Fatalf("events = %d, %v", len(evs), err)
	}
	return evs[len(evs)-1].Seq
}

// NFR-REL-001: after a power loss the database can be behind the transcript, a row
// saying idle while its transcript ends inside a turn. The start closes that turn from
// the transcript's end whatever the row says; a session at rest is left alone, and a
// second start changes nothing more.
func TestRecoverClosesATurnTheRowLost(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	// A session at rest, after a turn of its own.
	rest, _ := demoSession(t, e, "rest", "ask")
	e.write(rest.ID, event.TurnStarted, struct{}{})
	e.write(rest.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusRunning)})
	e.write(rest.ID, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})
	e.write(rest.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	e.write(rest.ID, event.Notice, event.NoticeData{Message: "after the turn"})
	restSeq := lastSeq(t, e, rest.ID)

	ses, _ := demoSession(t, e, "demo", "ask")
	before := lastSeq(t, e, ses.ID)
	e.Prompt(ctx, ses.ID, "go")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested && ev.SID == ses.ID })
	unsub()
	crash(e)

	// The database lost what it was told after the turn began.
	db, err := sql.Open("sqlite", filepath.Join(dir, "freeloop.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'idle', last_seq = ? WHERE id = ?`, before, ses.ID); err != nil {
		t.Fatal(err)
	}
	db.Close()

	e = openTest(t, dir)
	if got := lastTypes(t, e, ses.ID, 3); got != "permission_decided,turn_ended,session_status" {
		t.Errorf("the transcript ends with %s", got)
	}
	if got, _ := e.store.GetSession(ctx, ses.ID); got.Status != session.StatusIdle {
		t.Errorf("status = %s", got.Status)
	}
	if got := lastSeq(t, e, rest.ID); got != restSeq {
		t.Errorf("the session at rest was written to: seq %d, was %d", got, restSeq)
	}
	recovered := lastSeq(t, e, ses.ID)
	e.Close()

	// Once more: nothing is left to recover.
	e = openTest(t, dir)
	defer e.Close()
	if got := lastSeq(t, e, ses.ID); got != recovered {
		t.Errorf("a second start wrote to the recovered session: seq %d, was %d", got, recovered)
	}
	if got := lastSeq(t, e, rest.ID); got != restSeq {
		t.Errorf("a second start wrote to the session at rest: seq %d, was %d", got, restSeq)
	}
}

// NFR-REL-001, NFR-PERF-002: what the start looks at in each transcript is its end: a
// long session at rest costs a read of its end and is not written to, and a long one
// left in a turn is read back to that turn's start, not from its first line.
func TestRecoverReadsTheTranscriptsEnds(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	longTurn := func(sid string) {
		e.write(sid, event.TurnStarted, struct{}{})
		e.write(sid, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusRunning)})
		padChunks(e, sid, 3000)
		e.write(sid, event.TurnEnded, event.TurnEndedData{StopReason: "end_turn"})
		e.write(sid, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusIdle)})
	}
	rest, _ := demoSession(t, e, "rest", "ask")
	longTurn(rest.ID)
	e.write(rest.ID, event.Notice, event.NoticeData{Message: "after the turn"})
	failed, _ := demoSession(t, e, "failed", "ask")
	longTurn(failed.ID)
	e.write(failed.ID, event.TurnStarted, struct{}{})
	e.write(failed.ID, event.TurnEnded, event.TurnEndedData{StopReason: "error"})
	e.write(failed.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusError)})
	open, _ := demoSession(t, e, "open", "ask")
	longTurn(open.ID)
	e.write(open.ID, event.TurnStarted, struct{}{})
	e.write(open.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusRunning)})
	e.write(open.ID, event.PermissionRequested, map[string]string{"requestId": "r1", "toolCallId": "c1"})
	e.write(open.ID, event.PermissionRequested, map[string]string{"requestId": "r2", "toolCallId": "c2"})
	e.write(open.ID, event.PermissionDecided, map[string]string{"requestId": "r1", "toolCallId": "c1", "outcome": "allowed"})
	e.write(open.ID, event.SessionStatus, event.SessionStatusData{Status: string(session.StatusWaitingPermission)})
	restSeq, failedSeq := lastSeq(t, e, rest.ID), lastSeq(t, e, failed.ID)

	w := watchReads(e)
	e.recover(ctx)
	w.check(t, "recovery")
	if got := lastSeq(t, e, rest.ID); got != restSeq {
		t.Errorf("the session at rest was written to")
	}
	if got := lastSeq(t, e, failed.ID); got != failedSeq {
		t.Errorf("the failed session was written to")
	}
	if got := lastTypes(t, e, open.ID, 3); got != "permission_decided,turn_ended,session_status" {
		t.Errorf("the open turn ends with %s", got)
	}
	evs, _ := e.store.TailEvents(ctx, open.ID, 3)
	if !strings.Contains(string(evs[0].Data), `"r2"`) || !strings.Contains(string(evs[0].Data), "cancelled") {
		t.Errorf("decided %s", evs[0].Data)
	}
	if got, _ := e.store.GetSession(ctx, failed.ID); got.Status != session.StatusError {
		t.Errorf("the failed session's status became %s", got.Status)
	}
}
