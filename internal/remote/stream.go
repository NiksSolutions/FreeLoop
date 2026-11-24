package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// The phone follows sessions over server-sent events. A list's stream sends the
// whole summary each time something it shows changes, so a phone that lost its
// signal is right again at once; a session's stream sends its events in order,
// from where the phone left off. A phone with a stream open is being looked at and
// gets no push (notify.go); the phone app closes its streams while it is hidden.

const (
	keepAlive = 25 * time.Second
	settle    = 300 * time.Millisecond
	// writeWait is the longest one write to a stream, with its flush, may take.
	writeWait = 10 * time.Second
)

// stream is an event stream open to a phone. Each write, and each flush, must be done
// within writeWait: a phone that stopped reading, gone out of reach with its connection
// held open, ends the stream rather than holding it, and stops counting as looking.
type stream struct {
	w    http.ResponseWriter
	rc   *http.ResponseController
	wait time.Duration
}

func (st *stream) write(text string) bool {
	// Only a writer that is not the server's own, as in a test, takes no deadline.
	st.rc.SetWriteDeadline(time.Now().Add(st.wait))
	_, err := io.WriteString(st.w, text)
	return err == nil
}

func (st *stream) flush() bool {
	st.rc.SetWriteDeadline(time.Now().Add(st.wait))
	return st.rc.Flush() == nil
}

func (st *stream) send(text string) bool { return st.write(text) && st.flush() }

// openStream starts an event stream to phone d, or says the response cannot be one.
// While it is open the phone counts as looking, until closed is called.
func (s *Server) openStream(w http.ResponseWriter, d config.Device) (st *stream, closed func(), ok bool) {
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return nil, nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	st = &stream{w: w, rc: http.NewResponseController(w), wait: s.writeWait}
	if !st.flush() {
		return nil, nil, false
	}
	s.mu.Lock()
	if s.looking == nil {
		s.looking = map[string]int{}
	}
	s.looking[d.ID]++
	s.mu.Unlock()
	return st, func() {
		s.mu.Lock()
		if s.looking[d.ID]--; s.looking[d.ID] <= 0 {
			delete(s.looking, d.ID)
		}
		s.mu.Unlock()
	}, true
}

// changesSummary are the events after which the lists may read differently.
func changesSummary(t event.Type) bool {
	switch t {
	case event.SessionStatus, event.SessionInfoUpdate, event.PermissionRequested, event.PermissionDecided,
		event.TurnEnded, event.Error, event.Stuck:
		return true
	}
	return false
}

func (s *Server) summaryStream(w http.ResponseWriter, r *http.Request, d config.Device) {
	st, closed, ok := s.openStream(w, d)
	if !ok {
		return
	}
	defer closed()
	ch, cancel := s.subscribe()
	defer cancel()
	var sent []byte
	// send writes the summary; unless always, only when it changed since it was last
	// sent, and a ping otherwise.
	send := func(always bool) bool {
		sum, err := s.eng.PhoneSummary(r.Context())
		if err != nil {
			return false
		}
		b, _ := json.Marshal(sum)
		if !always && bytes.Equal(b, sent) {
			return st.send(": ping\n\n")
		}
		sent = b
		return st.send("event: summary\ndata: " + string(b) + "\n\n")
	}
	if !send(true) {
		return
	}
	ping := time.NewTicker(s.keepAlive)
	defer ping.Stop()
	var due <-chan time.Time
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if changesSummary(ev.Type) && due == nil {
				due = time.After(settle)
			}
		case <-due:
			due = nil
			if !send(true) {
				return
			}
		case <-ping.C:
			// The keep-alive reads the summary again: a change whose event the bus
			// dropped shows within a tick.
			if !send(false) {
				return
			}
		}
	}
}

func (s *Server) sessionStream(w http.ResponseWriter, r *http.Request, d config.Device) {
	id, ok := s.readable(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if last, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && last > after {
		after = last
	}
	// A phone opens a session at its end (issue 046): tail asks for only its last
	// events, and a stream taken up again goes on from where it was.
	if tail, _ := strconv.Atoi(r.URL.Query().Get("tail")); tail > 0 && after == 0 {
		evs, err := s.eng.EventsBefore(r.Context(), id, 0, tail)
		if err != nil {
			http.Error(w, "the session could not be read", http.StatusInternalServerError)
			return
		}
		if len(evs) > 0 {
			after = evs[0].Seq - 1
		}
	}
	// Subscribed before reading what came before, so nothing falls between.
	ch, cancel := s.subscribe()
	defer cancel()
	st, closed, ok := s.openStream(w, d)
	if !ok {
		return
	}
	defer closed()
	last := after
	write := func(evs []event.Event) bool {
		for _, ev := range evs {
			if ev.Seq <= last {
				continue
			}
			b, _ := json.Marshal(ev)
			if !st.write(fmt.Sprintf("id: %d\nevent: event\ndata: %s\n\n", ev.Seq, b)) {
				return false
			}
			last = ev.Seq
		}
		return st.flush()
	}
	// catchUp reads what the stream has not sent, as when the bus dropped some.
	catchUp := func() bool {
		for {
			evs, err := s.eng.Events(r.Context(), id, last, engine.MaxEventsPage)
			if err != nil || !write(evs) {
				return false
			}
			if len(evs) < engine.MaxEventsPage {
				return true
			}
		}
	}
	if !catchUp() {
		return
	}
	ping := time.NewTicker(s.keepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			if ev.SID != id || ev.Seq <= last {
				continue
			}
			if ev.Seq > last+1 {
				if !catchUp() {
					return
				}
				continue
			}
			if !write([]event.Event{ev}) {
				return
			}
		case <-ping.C:
			// The keep-alive catches up too: an event the bus dropped, with none after it
			// to show the gap, arrives within a tick.
			if !catchUp() || !st.send(": ping\n\n") {
				return
			}
		}
	}
}
