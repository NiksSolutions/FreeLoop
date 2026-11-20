package engine

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
)

// NFR-REL-003: a bug in background work is logged with its stack and kept to itself,
// as one in a turn is: the app and every other session go on.
func TestABackgroundPanicIsKeptToItself(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	e.goSafely("a test job", func() { panic("boom") })
	e.bg.Wait()
	if s := logged.String(); !strings.Contains(s, "a test job panicked") || !strings.Contains(s, "boom") || !strings.Contains(s, "safely_test.go") {
		t.Errorf("logged %q", s)
	}
	if _, err := e.ListSessions(context.Background()); err != nil {
		t.Errorf("the engine stopped with it: %v", err)
	}
}
