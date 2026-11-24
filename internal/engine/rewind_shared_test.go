package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// waitTurnOver waits until the session's runtime is no longer busy, which comes just
// after its turn_ended.
func waitTurnOver(t *testing.T, e *Engine, sid string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); e.turnRunning(sid); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s is still in a turn", sid)
		}
	}
}

// SES-014, SES-010: checkpoints snapshot the whole folder, so a rewind in a folder other
// sessions work in takes their work back too. It is refused while one of them is in a
// turn, naming it; the preview names those that changed files since the checkpoint, and
// after the rewind each of them has a notice saying which session rewound its files.
func TestRewindInASharedFolder(t *testing.T) {
	ctx := context.Background()
	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	e.mu.Lock()
	e.providers.built["bp"] = &configuredProvider{cfg: config.Provider{ID: "bp", Name: "bp", Type: "scripted"}, p: &byPrompt{}}
	e.mu.Unlock()
	root := t.TempDir()
	newSession := func(title, provider, mode string) string {
		ses, err := e.CreateSession(ctx, CreateSessionRequest{Title: title, ProjectRoot: root, ProviderID: provider, Model: "m", Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		return ses.ID
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}
	a := newSession("Rewinder", "bp", "accept_edits")
	e.Prompt(ctx, a, "write a.txt one")
	waitIdle(t, ch, a)
	// The rewind goes to before A's second message, as a rewind to before the first
	// writes a history_message without a message, which the schema refuses.
	e.Prompt(ctx, a, "write c.txt three")
	waitIdle(t, ch, a)
	evs, _ := e.Events(ctx, a, 0, 0)
	var seq int64
	for _, ev := range evs {
		if ev.Type == event.Checkpoint {
			seq = ev.Seq
		}
	}
	if seq == 0 {
		t.Fatal("no checkpoint")
	}
	// Another session in the same folder writes after the checkpoint; a third only talks.
	b := newSession("Other work", "bp", "accept_edits")
	e.Prompt(ctx, b, "write b.txt two")
	waitIdle(t, ch, b)
	quiet := newSession("Quiet", DemoProviderID, "ask")

	p, err := e.RewindPreview(ctx, a, seq)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Others) != 1 || p.Others[0].SessionID != b || p.Others[0].Title != "Other work" || p.Busy != "" {
		t.Errorf("preview = %+v", p)
	}

	// One in a turn holds the rewind back, and nothing changes.
	busy := newSession("Busy one", DemoProviderID, "ask")
	e.Prompt(ctx, busy, "go")
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.PermissionRequested && ev.SID == busy })
	if p, _ := e.RewindPreview(ctx, a, seq); p.Busy != "Busy one" {
		t.Errorf("preview with one in a turn = %+v", p)
	}
	if err := e.Rewind(ctx, a, seq); err == nil || !strings.Contains(err.Error(), "Busy one") {
		t.Fatalf("rewind beside a turn: %v", err)
	}
	if !exists("a.txt") || !exists("b.txt") || !exists("c.txt") {
		t.Fatal("a refused rewind changed files")
	}
	e.Cancel(busy)
	waitFor(t, ch, func(ev event.Event) bool { return ev.Type == event.TurnEnded && ev.SID == busy })
	waitTurnOver(t, e, busy)

	if err := e.Rewind(ctx, a, seq); err != nil {
		t.Fatal(err)
	}
	if !exists("a.txt") || exists("b.txt") || exists("c.txt") {
		t.Fatal("the rewind did not restore the folder")
	}
	notice := func(sid string) *event.NoticeData {
		evs, _ := e.Events(ctx, sid, 0, 0)
		for _, ev := range evs {
			var d event.NoticeData
			if ev.Type == event.Notice && json.Unmarshal(ev.Data, &d) == nil && d.RewoundBy != nil {
				return &d
			}
		}
		return nil
	}
	if d := notice(b); d == nil || d.RewoundBy.SessionID != a || d.RewoundBy.Title != "Rewinder" || !strings.Contains(d.Message, "Rewinder") {
		t.Errorf("the other session's notice: %+v", d)
	}
	// The session that asked and was cancelled ran nothing; the one that only talked
	// changed nothing: neither is told.
	if d := notice(busy); d != nil {
		t.Errorf("a session that wrote nothing was told: %+v", d)
	}
	if d := notice(quiet); d != nil {
		t.Errorf("a quiet session was told: %+v", d)
	}
	if d := notice(a); d != nil {
		t.Errorf("the rewinding session told itself: %+v", d)
	}
}
