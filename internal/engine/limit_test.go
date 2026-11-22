package engine

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// slow is a provider whose calls wait at a gate, counting how many run at
// once.
type slow struct {
	provider.Provider
	gate          chan struct{}
	running, peak atomic.Int32
}

func (s *slow) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.Event, error) {
	n := s.running.Add(1)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			break
		}
	}
	select {
	case <-s.gate:
	case <-ctx.Done():
	}
	ch, err := s.Provider.Stream(ctx, req)
	s.running.Add(-1)
	return ch, err
}

// OQ-013: a provider's cap holds across sessions, and a session that waits
// for a slot is shown as queued.
func TestProviderConcurrencyCap(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	sp := &slow{Provider: scripted.New("p", scripted.Step{Text: "ok"}), gate: make(chan struct{})}
	e.mu.Lock()
	e.providers.built["p"] = &configuredProvider{cfg: config.Provider{ID: "p", Name: "p", Type: "scripted"}, p: sp, slots: make(chan struct{}, 2)}
	e.mu.Unlock()
	var sids []string
	for range 5 {
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "p", Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		sids = append(sids, ses.ID)
	}
	queued := 0
	done := map[string]bool{}
	for _, sid := range sids {
		e.Prompt(ctx, sid, "go")
	}
	deadline := time.After(30 * time.Second)
	for len(done) < len(sids) {
		select {
		case ev := <-ch:
			if ev.Type == event.SessionStatus && strings.Contains(string(ev.Data), `"queued"`) {
				queued++
				// The two calls with a slot hold it at the gate until the other
				// three are seen waiting, however slowly the sessions start.
				if queued == 3 {
					close(sp.gate)
				}
			}
			if ev.Type == event.TurnEnded {
				done[ev.SID] = true
			}
		case <-deadline:
			t.Fatalf("sessions did not finish; %d were shown as queued", queued)
		}
	}
	if p := sp.peak.Load(); p > 2 {
		t.Errorf("%d calls ran at once on a provider capped at 2", p)
	}
}
