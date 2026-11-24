package engine

import (
	"context"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// OQ-053, issue 081: a session only read (viewed, exported, costed) does not keep its
// transcript open: the watch lets it go once unused. One with a runtime stays open.
func TestReadingASessionDoesNotKeepItsFileOpen(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	viewed, _ := demoSession(t, e, "viewed", "ask")
	running, _ := demoSession(t, e, "running", "ask")
	e.store.Release(viewed.ID)
	if _, err := e.EventsBefore(ctx, viewed.ID, 0, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Events(ctx, viewed.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	putIdleRuntime(e, running.ID)
	e.Events(ctx, running.ID, 0, 0)
	if !e.store.IsOpen(viewed.ID) || !e.store.IsOpen(running.ID) {
		t.Fatal("reads did not open the transcripts")
	}
	e.releaseTranscripts(time.Now())
	if !e.store.IsOpen(viewed.ID) {
		t.Error("let go of a transcript read a moment ago")
	}
	e.releaseTranscripts(time.Now().Add(2 * transcriptIdle))
	if e.store.IsOpen(viewed.ID) {
		t.Error("a session only read keeps its transcript open")
	}
	if !e.store.IsOpen(running.ID) {
		t.Error("a session with a runtime lost its open transcript")
	}
}

// Issue 081: deleting a session forgets its stuck watch and the record of its model
// calls.
func TestDeletingASessionForgetsItsState(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "gone", "ask")
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	e.contextMu.Lock()
	if e.contextSeen == nil {
		e.contextSeen = map[string][32]byte{}
	}
	e.contextSeen[ses.ID+"\x00main"] = [32]byte{1}
	e.contextMu.Unlock()
	e.stuck.mu.Lock()
	_, watched := e.stuck.sessions[ses.ID]
	e.stuck.mu.Unlock()
	if !watched {
		t.Fatal("the session has no stuck watch to forget")
	}
	if err := e.DeleteSession(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	e.stuck.mu.Lock()
	_, watched = e.stuck.sessions[ses.ID]
	e.stuck.mu.Unlock()
	e.contextMu.Lock()
	_, seen := e.contextSeen[ses.ID+"\x00main"]
	e.contextMu.Unlock()
	if watched || seen {
		t.Errorf("after delete: stuck watch %v, context record %v", watched, seen)
	}
}
