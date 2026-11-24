package engine

import (
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Issue 081: a subscriber that falls behind loses events rather than holding up the
// publisher, and finds a Missed marker after the events it has, even when nothing is
// published after the loss; once it reads, events come through again.
func TestASubscriberThatFellBehindLearnsOfIt(t *testing.T) {
	b := newBus()
	ch, cancel := b.subscribe(4)
	defer cancel()
	published := make(chan struct{})
	go func() {
		for i := range 10 {
			b.publish(event.Event{SID: "s", Seq: int64(i + 1), Type: event.Notice})
		}
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing blocked on a subscriber that does not read")
	}
	var got []event.Event
	for len(ch) > 0 {
		got = append(got, <-ch)
	}
	if len(got) != 4 || got[0].Seq != 1 || got[2].Seq != 3 || got[3].Type != Missed || got[3].SID != "" {
		t.Fatalf("read %+v", got)
	}
	b.publish(event.Event{SID: "s", Seq: 11, Type: event.Notice})
	if ev := <-ch; ev.Seq != 11 || ev.Type != event.Notice {
		t.Errorf("after reading, got %+v", ev)
	}

	// A subscriber that keeps up never sees one.
	b.publish(event.Event{SID: "s", Seq: 12, Type: event.Notice})
	if ev := <-ch; ev.Type == Missed {
		t.Error("a marker without a loss")
	}
}
