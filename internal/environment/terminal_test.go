package environment

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// screen collects everything a terminal prints, reading until it ends.
type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func watch(term Terminal) *screen {
	s := &screen{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := term.Read(b)
			s.mu.Lock()
			s.buf.Write(b[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitFor waits until the screen shows want. Git Bash on a Windows pseudo console
// can take more than ten seconds to start on a busy CI runner.
func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if strings.Contains(s.String(), want) {
			return
		}
	}
	t.Fatalf("no %q in the terminal's output:\n%s", want, s.String())
}

// testTerminalShell is the shell a test's terminal starts: /bin/sh, or on Windows the
// one the app uses there, Git Bash (OQ-043), as LoginShell finds it: none given.
func testTerminalShell() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []string{"/bin/sh", "-c"}
}

// GUI-012: a terminal is an interactive shell in the environment's folder,
// with the environment's variables, a size it can change, and an exit code.
func TestLocalTerminal(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLocal(LocalOptions{Root: dir, Guard: denyGuard{}, Shell: testTerminalShell(), Env: map[string]string{"FREELOOP_TERM_TEST": "from-the-env"}})
	if err != nil {
		t.Fatal(err)
	}
	term, err := l.OpenTerminal(context.Background(), TerminalSpec{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	// Even when a check fails: on Windows a shell left running holds the folder.
	t.Cleanup(func() { term.Close() })
	out := watch(term)
	// Two short lines: a pseudo console redraws a line wider than the screen.
	io.WriteString(term, "echo \"[$(pwd)]\"; echo \"vars: $FREELOOP_TERM_TEST $(stty size) $TERM $TERM_PROGRAM $COLORTERM\"\n")
	// GUI-054: a prompt can tell it runs in this app, and that it has colours.
	out.waitFor(t, "vars: from-the-env 30 100 xterm-256color Freeloop truecolor")
	if !strings.Contains(out.String(), "["+bashSpelling(l.Root())+"]") {
		t.Errorf("not in the folder:\n%s", out.String())
	}
	if err := term.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	io.WriteString(term, "stty size; exit 3\n")
	out.waitFor(t, "40 120")
	if code, err := term.Wait(); err != nil || code != 3 {
		t.Errorf("exit = %d, %v", code, err)
	}
	term.Close()
}

// Close ends the shell and what it started, even when the shell ignores
// the hangup.
func TestTerminalClose(t *testing.T) {
	l, err := NewLocal(LocalOptions{Root: t.TempDir(), Guard: denyGuard{}, Shell: testTerminalShell()})
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{"/bin/sh", "-c"}
	if runtime.GOOS == "windows" {
		argv = slices.Clone(LoginShell().NoStartup)
	}
	term, err := l.OpenTerminal(context.Background(), TerminalSpec{Argv: append(argv, "trap '' HUP; echo ready; sleep 60")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	watch(term).waitFor(t, "ready")
	start := time.Now()
	term.Close()
	if _, err := term.Wait(); err != nil && !strings.Contains(err.Error(), "signal") {
		t.Errorf("wait: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("Close took too long")
	}
}
