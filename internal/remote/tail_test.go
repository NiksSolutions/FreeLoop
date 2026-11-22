package remote

import (
	"bufio"
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

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
