package store

import (
	"context"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// OQ-053: a released session holds no open file, and the next event opens it
// again and carries on from the last seq.
func TestReleaseClosesTheTranscriptAndTheNextAppendOpensIt(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	a, err := s.CreateSession(ctx, session.Session{Title: "idle", HarnessID: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(ctx, a.ID, event.Notice, text("one")); err != nil {
		t.Fatal(err)
	}
	open := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, ok := s.files[a.ID]
		return ok
	}
	if !open() {
		t.Fatal("the transcript is not open after an append")
	}
	s.Release(a.ID)
	if open() {
		t.Fatal("the transcript is still open after Release")
	}
	s.Release(a.ID) // a session with nothing open is fine
	ev, err := s.Append(ctx, a.ID, event.Notice, text("two"))
	if err != nil || ev.Seq != 2 {
		t.Fatalf("append after release = %+v, %v", ev, err)
	}
	if evs, _ := s.Events(ctx, a.ID, 0); len(evs) != 2 {
		t.Errorf("events = %d", len(evs))
	}
}

// OQ-053, issue 081: a transcript only read is let go once unused, unless the caller
// keeps its session, and one used since the cutoff stays.
func TestUnusedTranscriptsAreReleased(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir(), nil)
	defer s.Close()
	var ids []string
	for _, title := range []string{"viewed", "exported", "running"} {
		ses, err := s.CreateSession(ctx, session.Session{Title: title, HarnessID: "builtin"})
		if err != nil {
			t.Fatal(err)
		}
		s.Append(ctx, ses.ID, event.Notice, text("one"))
		ids = append(ids, ses.ID)
	}
	s.Release(ids[0])
	s.Release(ids[1])
	open := func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.files)
	}
	// Reads open the transcripts again.
	s.EventsLimit(ctx, ids[0], 0, 10)
	s.TailEvents(ctx, ids[1], 5)
	if n := open(); n != 3 {
		t.Fatalf("open after reads = %d", n)
	}
	if n := s.ReleaseUnused(time.Now().Add(-time.Minute), nil); n != 0 || open() != 3 {
		t.Errorf("released %d used within the minute", n)
	}
	keep := func(sid string) bool { return sid == ids[2] }
	if n := s.ReleaseUnused(time.Now().Add(time.Second), keep); n != 2 || open() != 1 || !s.IsOpen(ids[2]) {
		t.Errorf("released %d, %d open, the kept one open %v", n, open(), s.IsOpen(ids[2]))
	}
	// A released one reads and appends as before.
	if ev, err := s.Append(ctx, ids[0], event.Notice, text("two")); err != nil || ev.Seq != 2 {
		t.Errorf("append after release = %+v, %v", ev, err)
	}
}
