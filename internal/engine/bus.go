package engine

import (
	"sync"

	"github.com/FinanceNik/Freeloop/internal/event"
)

// Missed is what the bus sends a subscriber that lost events for want of room in its
// buffer: an event of this type, with no session and no seq, which is never written to
// a transcript. The subscriber reads again, from the store, the events after the last
// one it has of each session it follows (issue 081).
const Missed event.Type = "events_missed"

var missed = event.Event{V: event.Version, Type: Missed, Data: []byte("{}")}

// bus fans events out to subscribers without ever blocking the publisher.
type bus struct {
	mu     sync.Mutex
	subs   map[chan event.Event]struct{}
	closed bool
}

func newBus() *bus {
	return &bus{subs: map[chan event.Event]struct{}{}}
}

// subscribe opens a subscription with a buffer of buf events, the last place of which
// is kept for Missed.
func (b *bus) subscribe(buf int) (<-chan event.Event, func()) {
	ch := make(chan event.Event, max(buf, 2))
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if _, ok := b.subs[ch]; ok {
				delete(b.subs, ch)
				close(ch)
			}
		})
	}
}

// publish delivers ev to every subscriber with room in its buffer and drops it for
// the rest. An event fills a buffer only up to its last place, which a drop gives to
// Missed: the subscriber finds it after the events it has, so it learns of the loss
// even if nothing follows. A buffer full to the last place already ends in one, since
// only Missed takes that place and only publish, under b.mu, sends.
func (b *bus) publish(ev event.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		next := ev
		if len(ch) >= cap(ch)-1 {
			next = missed
		}
		select {
		case ch <- next:
		default:
		}
	}
}

func (b *bus) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
