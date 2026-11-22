package engine

import (
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Issue 046, NFR-PERF-001: a session opens at its end and reads what came before a
// page at a time, and no read gives more than a page, however much is asked for.
func TestEventsBeforeReadsBackAPageAtATime(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	have, err := e.Events(ctx, ses.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for range MaxEventsPage + 10 - len(have) {
		if _, err := e.store.Append(ctx, ses.ID, event.Notice, []byte(`{"message":"x"}`)); err != nil {
			t.Fatal(err)
		}
	}
	last := int64(MaxEventsPage + 10)
	seqs := func(evs []event.Event) (int64, int64, int) {
		if len(evs) == 0 {
			return 0, 0, 0
		}
		for i := 1; i < len(evs); i++ {
			if evs[i].Seq != evs[i-1].Seq+1 {
				t.Fatalf("a gap after seq %d", evs[i-1].Seq)
			}
		}
		return evs[0].Seq, evs[len(evs)-1].Seq, len(evs)
	}
	for _, c := range []struct {
		name          string
		before        int64
		limit         int
		first, end    int64
		n             int
		wantNothingAt bool
	}{
		{name: "the end", before: 0, limit: 5, first: last - 4, end: last, n: 5},
		{name: "the page before", before: last - 4, limit: 5, first: last - 9, end: last - 5, n: 5},
		{name: "the start, short", before: 3, limit: 5, first: 1, end: 2, n: 2},
		{name: "nothing before the first", before: 1, limit: 5, wantNothingAt: true},
		{name: "no limit is a page", before: 0, limit: 0, first: last - MaxEventsPage + 1, end: last, n: MaxEventsPage},
		{name: "more than a page is a page", before: last + 1, limit: 1_000_000, first: last - MaxEventsPage + 1, end: last, n: MaxEventsPage},
	} {
		evs, err := e.EventsBefore(ctx, ses.ID, c.before, c.limit)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		first, end, n := seqs(evs)
		if c.wantNothingAt {
			if n != 0 {
				t.Errorf("%s: %d events", c.name, n)
			}
			continue
		}
		if first != c.first || end != c.end || n != c.n {
			t.Errorf("%s: seqs %d to %d (%d), want %d to %d (%d)", c.name, first, end, n, c.first, c.end, c.n)
		}
	}
	// What the window's forward reads give is a page too.
	if evs, err := e.Events(ctx, ses.ID, 0, MaxEventsPage); err != nil || len(evs) != MaxEventsPage {
		t.Errorf("a forward page: %d events, %v", len(evs), err)
	}
}
