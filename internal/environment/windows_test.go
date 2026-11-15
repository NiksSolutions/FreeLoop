package environment

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These run only on Windows: the CI job's runner has Git Bash (OQ-043).

// junction makes link a junction to target, which needs no privilege on Windows.
func junction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v: %s", err, out)
	}
}

// A folder swapped for a junction between the check and the open is not read or
// written through: the opened file is not where the guard looked.
func TestWindowsJunctionSwappedInIsRefused(t *testing.T) {
	skipUnlessWindows(t)
	root := Canonical(t.TempDir())
	outside := Canonical(t.TempDir())
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "secret.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{root}})
	if err != nil {
		t.Fatal(err)
	}
	swap := func() {
		os.RemoveAll(filepath.Join(root, "sub"))
		junction(t, filepath.Join(root, "sub"), outside)
	}
	t.Cleanup(func() { afterCheck = func() {} })

	afterCheck = swap
	if b, err := l.FS().ReadFile("sub/secret.txt"); err == nil {
		t.Fatalf("read through a junction swapped in: %q", b)
	}

	afterCheck = func() {}
	os.RemoveAll(filepath.Join(root, "sub"))
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	afterCheck = swap
	if err := l.FS().WriteFile("sub/new.txt", []byte("x")); err == nil {
		t.Fatal("wrote through a junction swapped in")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Error("the file made through the junction was left outside the project")
	}
}

// A plain write and read work, and an existing file keeps nothing of its old content.
func TestWindowsWriteRead(t *testing.T) {
	skipUnlessWindows(t)
	root := Canonical(t.TempDir())
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{root}})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.FS().WriteFile("a/b/c.txt", []byte("a longer first version")); err != nil {
		t.Fatal(err)
	}
	if err := l.FS().WriteFile("a/b/c.txt", []byte("short")); err != nil {
		t.Fatal(err)
	}
	if b, err := l.FS().ReadFile(`a\b\c.txt`); err != nil || string(b) != "short" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
}

// What a command leaves running in the background ends when the session stops its
// lingering groups, and a command that runs past its timeout takes its children with
// it: both through the command's job.
func TestWindowsJobs(t *testing.T) {
	skipUnlessWindows(t)
	root := Canonical(t.TempDir())
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{root}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := l.Exec(ctx, ExecSpec{Command: `sleep 300 >/dev/null 2>&1 & echo $!`})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("Exec: %v %+v", err, res)
	}
	l.lingerMu.Lock()
	n := len(l.lingering)
	l.lingerMu.Unlock()
	if n != 1 {
		t.Fatalf("%d lingering groups, want 1", n)
	}
	l.StopLingering()
	l.lingerMu.Lock()
	n = len(l.lingering)
	l.lingerMu.Unlock()
	if n != 0 {
		t.Fatalf("%d lingering groups after StopLingering", n)
	}

	start := time.Now()
	res, err = l.Exec(ctx, ExecSpec{Command: `sleep 300 & sleep 300`, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || time.Since(start) > 30*time.Second {
		t.Fatalf("timed out %v after %s", res.TimedOut, time.Since(start))
	}
}

// Commands run in Git Bash, with the variables Windows programs need.
func TestWindowsShell(t *testing.T) {
	skipUnlessWindows(t)
	if LoginShell().Path == "bash.exe" {
		t.Skip("no Git Bash on this machine")
	}
	root := Canonical(t.TempDir())
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{root}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := l.Exec(context.Background(), ExecSpec{Command: `echo "$BASH_VERSION|$SYSTEMROOT"`})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("Exec: %v %+v", err, res)
	}
	ver, sysroot, _ := strings.Cut(strings.TrimSpace(res.Output), "|")
	if ver == "" || sysroot == "" {
		t.Errorf("output %q: want bash's version and SYSTEMROOT", res.Output)
	}
}

// Security review, 2026-11-13: a share on another computer named by a tool call is
// not looked at while the call is judged, since looking opens an SMB connection that
// sends the user's NTLM hash, and outside the project it is not read or written even
// when allowed (TestFarPath has the spellings).
func TestWindowsShareOutsideTheProjectIsNotTouched(t *testing.T) {
	skipUnlessWindows(t)
	root := Canonical(t.TempDir())
	l, err := NewLocal(LocalOptions{Root: root, Guard: denyGuard{}})
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		`\\attacker.invalid\s\x`:         `\\attacker.invalid\s\x`,
		`//attacker.invalid/s/x`:         `\\attacker.invalid\s\x`,
		`\\?\UNC\attacker.invalid\s\x`:   `\\attacker.invalid\s\x`,
		`\\.\pipe\freeloop-test-nothing`: `\\.\pipe\freeloop-test-nothing`,
	} {
		if got := l.Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
		if l.Contains(in) {
			t.Errorf("%s is in the project", in)
		}
		if got := Spellings(in); got[len(got)-1] != filepath.Clean(want) {
			t.Errorf("Spellings(%q) = %q", in, got)
		}
		if _, err := l.FS().ReadFile(in); !errors.Is(err, ErrProtected) {
			t.Errorf("ReadFile(%s) = %v", in, err)
		}
		if _, err := l.FS().Stat(in); !errors.Is(err, ErrProtected) {
			t.Errorf("Stat(%s) = %v", in, err)
		}
		if err := l.FS().WriteFile(in, []byte("x")); !errors.Is(err, ErrProtected) {
			t.Errorf("WriteFile(%s) = %v", in, err)
		}
	}
}
