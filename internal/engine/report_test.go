package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

func TestProblemReportCarriesWhatAnIssueNeedsAndNoSecrets(t *testing.T) {
	dir, home := t.TempDir(), t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: home, Dev: true, WatchInterval: -1,
		SystemInfo: func() string { return "macOS 15.1 (24B83), Mac15,3, Apple M3, 16 GB" }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	secret := "sk-report-FAKE-0123456789abcdef"
	e.remember(secret)

	// A log of 250 lines, a crash trace among the last with the secret in it,
	// written straight to the file as a crash would be.
	var log strings.Builder
	for i := 1; i <= 250; i++ {
		fmt.Fprintf(&log, "2026/11/08 12:00:00 line %d\n", i)
	}
	fmt.Fprintf(&log, "panic: request with key %s failed\n\t%s/Documents/project/main.go:12\n", secret, home)
	if err := os.MkdirAll(filepath.Join(dir, config.LogsDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.LogPath(dir), []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	ses, _ := demoSession(t, e, "report", "ask")

	got, err := e.ProblemReport(context.Background(), ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Freeloop " + AppVersion,
		"System: macOS 15.1 (24B83), Mac15,3, Apple M3, 16 GB",
		"Session: Built-in on Scripted demo, model scripted, ask mode",
		"The log's last 200 lines:",
		"line 250\n",
		"~/Documents/project/main.go:12",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "line 52\n") || !strings.Contains(got, "line 53\n") {
		t.Error("the report does not hold exactly the log's last 200 lines")
	}
	if strings.Contains(got, secret) || strings.Contains(got, home) {
		t.Errorf("the report holds the secret or the home folder:\n%s", got[strings.Index(got, "panic"):])
	}
	if strings.Contains(got, "report") && strings.Contains(got, "Session: report") {
		t.Error("the report names the session's title")
	}

	// Without a session, and with no log, it still says what it can.
	os.Remove(config.LogPath(dir))
	got, err = e.ProblemReport(context.Background(), "")
	if err != nil || strings.Contains(got, "Session:") || !strings.Contains(got, "The log's last 0 lines") {
		t.Errorf("no session and no log = %q, %v", got, err)
	}
}

// GUI-046: a report says what the app's own goroutines are doing, by where they
// stand, so an app that seems stuck can be seen from its report; without addresses
// or values, and within a bound.
func TestProblemReportSaysWhatTheAppIsDoing(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	go parkedForTheReport(started, release)
	<-started
	got, err := e.ProblemReport(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, doing, ok := strings.Cut(got, "What the app is doing")
	if !ok {
		t.Fatalf("no goroutines in the report:\n%s", got)
	}
	doing, _, _ = strings.Cut(doing, "The log's last")
	// With its file whole, whatever the folders on the way are called (a space in
	// "Application Support", say).
	_, file, _, _ := goruntime.Caller(0)
	if !strings.Contains(doing, "engine.parkedForTheReport ("+e.anonymize(file)+":") {
		t.Errorf("the parked goroutine is missing, or its file is cut short:\n%s", doing)
	}
	if strings.Contains(doing, "0x") {
		t.Errorf("addresses in the report:\n%s", doing)
	}
	if n := strings.Count(doing, "\n"); n > reportGoroutineLines+5 {
		t.Errorf("%d lines", n)
	}
}

func parkedForTheReport(started, release chan struct{}) {
	close(started)
	<-release
}

// GUI-046: a report says how far behind the transcripts each count is, so one that
// stopped moving can be seen from the report.
func TestProblemReportSaysHowFarTheCountsAreBehind(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ses, _ := demoSession(t, e, "behind", "ask")
	e.emit(ses.ID, event.TurnStarted, struct{}{})
	got, err := e.ProblemReport(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, counts, ok := strings.Cut(got, "Counts kept from the transcripts")
	if !ok {
		t.Fatalf("no counts in the report:\n%s", got)
	}
	counts, _, _ = strings.Cut(counts, "\n\n")
	if !strings.Contains(counts, "commits and their usage: 1 session, ") || !strings.Contains(counts, "brought up to date when the Usage page asks") {
		t.Errorf("counts:\n%s", counts)
	}
}
