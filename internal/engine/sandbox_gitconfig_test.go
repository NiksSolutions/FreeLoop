//go:build !windows

package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// passConfiner runs commands as they are, standing in for a real sandbox where
// only the variables that go with confined commands are under test.
type passConfiner struct{}

func (passConfiner) Confine(_ environment.Confinement, argv []string) ([]string, error) {
	return argv, nil
}

// The sandbox's copy of the user's git name sat where sandboxed commands write,
// and GIT_CONFIG_GLOBAL naming it went to everything the environment starts
// unconfined: the terminal, agents and MCP servers. A command that needs no
// approval could write core.fsmonitor into it and have the user's next git status
// run it outside the sandbox; or swap the file for a link, which the next session
// then wrote through (security review, 2026-11-13).
func TestSandboxGitConfigStaysWithConfinedCommands(t *testing.T) {
	ctx := context.Background()
	home := environment.Canonical(t.TempDir())
	repo := filepath.Join(home, "code", "repo")
	os.MkdirAll(repo, 0o755)
	gitIn(t, repo, "init", "-q")
	e, err := Open(Options{DataDir: filepath.Join(home, "data"), Secrets: secrets.NewMemory(), Home: home, Confiner: passConfiner{}, WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.loginOnce.Do(func() {
		e.loginEnv = map[string]string{}
		e.loginGitConfig = "[user]\n\tname = \"Sandbox Name\"\n"
	})
	p, err := e.SetProjectTrust(ctx, repo, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Environment{ID: "sandboxed", Kind: "local", Sandbox: &config.Sandbox{Enabled: true}}
	open := func(id string) *environment.Local {
		env, done, err := e.sandboxEnv(ctx, session.Session{ID: id, ProjectID: p.ID, ProjectRoot: repo}, cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(done)
		return env
	}
	env := open("ses_a")
	run := func(spec environment.ExecSpec) string {
		t.Helper()
		spec.Timeout = 20 * time.Second
		res, err := env.Exec(ctx, spec)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(res.Output)
	}

	// Confined commands get the copy, and git reads the name from it.
	own := run(environment.ExecSpec{Command: `printf %s "$GIT_CONFIG_GLOBAL"`})
	if own == "" || !strings.HasPrefix(own, e.projectCache(p.ID)) {
		t.Fatalf("a confined command's GIT_CONFIG_GLOBAL = %q", own)
	}
	if name := run(environment.ExecSpec{Command: "git config --global --get user.name"}); name != "Sandbox Name" {
		t.Errorf("git in the sandbox reads user.name %q", name)
	}

	// Nothing unconfined is pointed at it: not a command outside the sandbox,
	// and not what Spawn starts (agents, MCP servers), which the terminal shares.
	if v := run(environment.ExecSpec{Command: `printf %s "$GIT_CONFIG_GLOBAL"`, OutsideSandbox: true}); v != "" {
		t.Errorf("an unconfined command has GIT_CONFIG_GLOBAL=%q", v)
	}
	proc, err := env.Spawn(ctx, environment.SpawnSpec{Argv: []string{"/bin/sh", "-c", `printf %s "$GIT_CONFIG_GLOBAL"`}})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(proc.Stdout())
	proc.Wait()
	if len(out) != 0 {
		t.Errorf("a spawned process has GIT_CONFIG_GLOBAL=%q", out)
	}

	// A link planted in its place is replaced, not written through.
	victim := filepath.Join(home, ".bashrc")
	os.WriteFile(victim, []byte("users own file\n"), 0o644)
	os.Remove(own)
	if err := os.Symlink(victim, own); err != nil {
		t.Fatal(err)
	}
	open("ses_b")
	if b, _ := os.ReadFile(victim); string(b) != "users own file\n" {
		t.Errorf("the next session wrote through a planted link: %q", b)
	}
	if fi, err := os.Lstat(own); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("the copy is not a file of its own: %v, %v", fi, err)
	}

	// A scratch folder replaced by a link is not followed by the next session:
	// its commands would write wherever the link leads.
	elsewhere := filepath.Join(home, "Documents")
	os.MkdirAll(elsewhere, 0o755)
	cache := filepath.Dir(own)
	tmp := filepath.Join(filepath.Dir(cache), "tmp")
	for _, d := range []string{cache, tmp} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, d); err != nil {
			t.Fatal(err)
		}
	}
	env = open("ses_c")
	for _, v := range []string{"XDG_CACHE_HOME", "TMPDIR"} {
		if got := run(environment.ExecSpec{Command: `printf %s "$` + v + `"`}); strings.HasPrefix(got, elsewhere) || !strings.HasPrefix(got, e.projectCache(p.ID)) {
			t.Errorf("%s = %q after its folder was replaced by a link", v, got)
		}
	}
	for _, d := range []string{cache, tmp} {
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
			t.Errorf("%s is not a folder of its own: %v, %v", d, fi, err)
		}
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "gitconfig")); err == nil {
		t.Error("the copy of the git config was written through the link")
	}
}
