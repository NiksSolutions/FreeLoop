//go:build !windows

package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// ENV-009: what the login shell sets is read again after a read that failed, a slow
// shell at a cold start, say, rather than left out of every sandboxed command until
// the app restarts; and one caller's cancelled request does not spoil it for the rest.
func TestTheLoginShellIsReadAgainAfterAFailure(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	ctx := context.Background()
	defer func(read func(*Engine, context.Context, *environment.Local) (map[string]string, string, error)) {
		readLoginShell = read
	}(readLoginShell)
	defer func(due func(time.Time) bool) { loginRetryDue = due }(loginRetryDue)
	loginRetryDue = func(time.Time) bool { return true }
	calls := 0
	readLoginShell = func(*Engine, context.Context, *environment.Local) (map[string]string, string, error) {
		calls++
		if calls == 1 {
			return nil, "", errors.New("the shell took too long")
		}
		return map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin"}, "[user]\n\tname = \"A\"\n", nil
	}
	if env, git := e.loginVars(ctx, nil); len(env) != 0 || git != "" {
		t.Fatalf("a failed read gave %v, %q", env, git)
	}
	if env, git := e.loginVars(ctx, nil); env["PATH"] != "/opt/homebrew/bin:/usr/bin" || git == "" {
		t.Fatalf("the read after it gave %v, %q", env, git)
	}
	e.loginVars(ctx, nil)
	if calls != 2 {
		t.Errorf("read %d times; once it worked it is kept", calls)
	}
}

func TestACancelledCallerStillGetsTheLoginShell(t *testing.T) {
	e := openTest(t, t.TempDir())
	defer e.Close()
	host, err := e.localEnv(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if env, _ := e.loginVars(cancelled, host); env["PATH"] == "" {
		t.Errorf("a cancelled first caller left the login shell unread: %v", env)
	}
}
