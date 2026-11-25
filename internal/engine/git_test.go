package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestParseStatus(t *testing.T) {
	st := parseStatus("# branch.oid 1234abcd\n# branch.head feature/x\n# branch.upstream origin/feature/x\n1 .M N... 100644 100644 100644 a b go.mod\n? new.txt\n")
	if !st.Repo || st.Head != "1234abcd" || st.Branch != "feature/x" || st.Dirty != 2 {
		t.Errorf("status = %+v", st)
	}
	if st := parseStatus("# branch.oid (initial)\n# branch.head (detached)\n"); st.Head != "" || st.Branch != "" || st.Dirty != 0 {
		t.Errorf("initial/detached = %+v", st)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "first")
	run("remote", "add", "origin", "https://example.com/r.git")
	return dir
}

func TestGitAgainstARealRepo(t *testing.T) {
	dir := gitRepo(t)
	home := t.TempDir()
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st := gitState(ctx, env, true)
	if !st.Repo || st.Branch != "main" || len(st.Head) != 40 || st.Dirty != 0 {
		t.Errorf("clean repo = %+v", st)
	}
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	if st := gitState(ctx, env, true); st.Dirty != 1 {
		t.Errorf("dirty = %d, want 1", st.Dirty)
	}
	// An untrusted project gets branch and HEAD from plumbing only.
	if u := gitState(ctx, env, false); !u.Repo || u.Dirty != -1 || u.Branch != "main" || u.Head != st.Head {
		t.Errorf("untrusted = %+v", u)
	}
	first, remote := fingerprint(ctx, env)
	if first != st.Head || remote != "https://example.com/r.git" {
		t.Errorf("fingerprint = %q, %q", first, remote)
	}

	plain := t.TempDir()
	penv, _ := environment.NewLocal(environment.LocalOptions{Root: plain, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(plain)})
	if st := gitState(ctx, penv, true); st.Repo {
		t.Errorf("not a repo = %+v", st)
	}
}

// OQ-046: the query a git asks of its system config, here a stand-in for Git for
// Windows' file, prints what systemGitFrom reads; a missing file sets nothing.
func TestSystemGitConfigReadByGit(t *testing.T) {
	dir := gitRepo(t)
	home := t.TempDir()
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	query := func(system string) systemGit {
		t.Helper()
		res, err := env.Exec(context.Background(), environment.ExecSpec{Argv: gitArgv(env, systemGitQuery...), Env: gitEnv(map[string]string{"GIT_CONFIG_SYSTEM": system}), SeparateStderr: true})
		if err != nil || res.ExitCode > 1 {
			t.Fatalf("the query: %v %+v", err, res)
		}
		return systemGitFrom(res.Output)
	}
	write := func(name, text string) string {
		p := filepath.Join(home, name)
		os.WriteFile(p, []byte(text), 0o644)
		return p
	}
	gfw := "[diff \"astextplain\"]\n\ttextconv = astextplain\n[filter \"lfs\"]\n\tclean = git-lfs clean -- %f\n\tprocess = git-lfs filter-process\n" +
		"[http]\n\tsslBackend = openssl\n[core]\n\tautocrlf = true\n\tfscache = true\n\tsymlinks = false\n" +
		"[credential]\n\thelper =\n\thelper = C:/Program\\\\ Files/Git/mingw64/bin/git-credential-manager.exe\n[credential \"https://dev.azure.com\"]\n\tuseHttpPath = true\n"
	got := query(write("gitconfig", gfw))
	if !got.Narrow || got.AutoCRLF != "true" || !slices.Equal(got.Helpers, []string{`C:/Program\ Files/Git/mingw64/bin/git-credential-manager.exe`}) {
		t.Errorf("Git for Windows' system config gives %+v", got)
	}
	if got := query(write("manager", "[core]\n\tautocrlf = input\n[credential]\n\thelper = manager\n")); !got.Narrow || got.AutoCRLF != "input" || !slices.Equal(got.Helpers, []string{"manager"}) {
		t.Errorf("helper = manager gives %+v", got)
	}
	// A helper in a file the system config includes counts as the system's.
	write("more", "[credential]\n\thelper = wincred\n")
	if got := query(write("includes", "[credential]\n\thelper = manager\n[include]\n\tpath = more\n")); got.Narrow {
		t.Errorf("an included helper was missed: %+v", got)
	}
	if got := query(write("addressed", "[credential]\n\thelper = manager\n[credential \"https://example.com\"]\n\thelper = !f() { echo password=x; }; f\n")); got.Narrow {
		t.Errorf("a helper for one address was missed: %+v", got)
	}
	if got := query(filepath.Join(home, "missing")); got.Narrow || got.AutoCRLF != "" || got.Helpers != nil {
		t.Errorf("no system config gives %+v", got)
	}
}

// OQ-046 (b), with a real git: what Windows' system config gives the app's git reaches
// it on the command line, core.autocrlf only where neither the command nor the user's
// own config sets it, since a -c wins over both and git reads the system's value below
// them. The system's own file is out of it (GIT_CONFIG_NOSYSTEM), on this machine too.
func TestSystemGitConfigReachesGit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home) // the user's global config, empty for now
	dir := gitRepo(t)
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sys := systemGit{Narrow: true, AutoCRLF: "true", Helpers: []string{"manager"}}
	helper := []string{"-c", "credential.helper=manager"}
	both := append(slices.Clone(helper), "-c", "core.autocrlf=true")
	expect := func(args, want []string) {
		t.Helper()
		if got := systemGitArgs(ctx, env, sys, args); !slices.Equal(got, want) {
			t.Errorf("systemGitArgs(%q) = %q, want %q", args, got, want)
		}
	}
	expect([]string{"status", "--porcelain"}, both)
	expect([]string{"rev-parse", "HEAD"}, helper) // reads no line endings, so is not asked about
	expect([]string{"push", "--porcelain", "origin", "HEAD"}, helper)
	expect([]string{"-c", "core.autocrlf=false", "status"}, helper)
	if got := systemGitArgs(ctx, env, systemGit{}, []string{"status"}); got != nil {
		t.Errorf("(a) passes %q", got)
	}

	// A file Git for Windows checked out with \r\n where the commit has \n shows as
	// changed once its time changes, unless git gets the system's core.autocrlf.
	lines := filepath.Join(dir, "lines.txt")
	os.WriteFile(lines, []byte("one\ntwo\n"), 0o644)
	gitIn(t, dir, "add", "lines.txt")
	gitIn(t, dir, "commit", "-q", "-m", "lines")
	os.Remove(lines)
	gitIn(t, dir, "-c", "core.autocrlf=true", "checkout", "--", "lines.txt")
	if b, _ := os.ReadFile(lines); string(b) != "one\r\ntwo\r\n" {
		t.Fatalf("checked out as %q", b)
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(lines, later, later)
	status := func(more []string) string {
		t.Helper()
		res, err := env.Exec(ctx, environment.ExecSpec{Argv: gitArgv(env, append(more, "status", "--porcelain")...), Env: gitEnv(nil)})
		if err != nil || res.ExitCode != 0 {
			t.Fatalf("git status: %v %+v", err, res)
		}
		return res.Output
	}
	if out := status(nil); !strings.Contains(out, "lines.txt") {
		t.Fatalf("without core.autocrlf the line endings are no change: %q", out)
	}
	if out := status(systemGitArgs(ctx, env, sys, []string{"status", "--porcelain"})); out != "" {
		t.Errorf("with the system's core.autocrlf: %q", out)
	}

	// The repository's own setting wins over the system's, as in git, and so does one
	// in a repository the command names with -C, and the user's global one.
	gitIn(t, dir, "config", "core.autocrlf", "false")
	expect([]string{"status"}, helper)
	other := gitRepo(t)
	expect([]string{"-C", other, "status"}, both)
	gitIn(t, other, "config", "core.autocrlf", "input")
	expect([]string{"-C", other, "status"}, helper)
	gitIn(t, dir, "config", "--unset", "core.autocrlf")
	expect([]string{"status"}, both)
	os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\tautocrlf = input\n"), 0o644)
	expect([]string{"status"}, helper)

	// manager is git-credential-manager beside git, which signs in a push: here a
	// stand-in on PATH. Git for Windows would find its own first.
	if goruntime.GOOS != "windows" {
		bin := t.TempDir()
		os.WriteFile(filepath.Join(bin, "git-credential-manager"), []byte("#!/bin/sh\n[ \"$1\" = get ] && printf 'username=u\\npassword=from-gcm\\n'\nexit 0\n"), 0o755)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		fill := []string{"credential", "fill"}
		res, err := env.Exec(ctx, environment.ExecSpec{Argv: gitArgv(env, append(systemGitArgs(ctx, env, sys, fill), fill...)...), Env: gitEnv(nil), Stdin: []byte("protocol=https\nhost=example.com\n\n")})
		if err != nil || !strings.Contains(res.Output, "password=from-gcm") {
			t.Errorf("git credential fill: %v %q", err, res.Output)
		}
	}
	// gh's own git calls get the helper through the environment, and nothing from the
	// system's file.
	res, err := env.Exec(ctx, environment.ExecSpec{Argv: []string{"git", "config", "--get-all", "credential.helper"}, Env: pushEnvOn("windows", sys, [][2]string{{"credential.helper", "manager"}})})
	if err != nil || strings.TrimSpace(res.Output) != "manager" {
		t.Errorf("gh's git sees the helpers %q (%v)", res.Output, err)
	}
}

// OQ-046 (b): the system's TLS settings reach the commands that talk to a remote,
// below the user's own as git ranks them: a setting the user's config has is not
// passed on the command line, where it would win.
func TestSystemGitTLSReachesNetworkCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := gitRepo(t)
	env, err := environment.NewLocal(environment.LocalOptions{Root: dir, Guard: permission.NewPolicy(home, filepath.Join(home, "d")).Guard(dir)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	bundle := "C:/Program Files/Git/mingw64/etc/ssl/certs/ca-bundle.crt"
	sys := systemGit{Narrow: true, Helpers: []string{"manager"}, TLS: [][2]string{{"http.sslbackend", "openssl"}, {"http.sslcainfo", bundle}}}
	helper := []string{"-c", "credential.helper=manager"}
	expect := func(args, want []string) {
		t.Helper()
		if got := systemGitArgs(ctx, env, sys, args); !slices.Equal(got, want) {
			t.Errorf("systemGitArgs(%q) = %q, want %q", args, got, want)
		}
	}
	expect([]string{"push", "--porcelain", "origin", "HEAD"}, append(slices.Clone(helper), "-c", "http.sslbackend=openssl", "-c", "http.sslcainfo="+bundle))
	expect([]string{"status"}, helper) // talks to no remote
	os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[http]\n\tsslCAInfo = /etc/mine.pem\n"), 0o644)
	expect([]string{"push", "origin", "HEAD"}, append(slices.Clone(helper), "-c", "http.sslbackend=openssl"))
	if vars := pushEnvOn("windows", sys, systemGitPairs(ctx, env, sys, nil, true, true)); vars["GIT_CONFIG_KEY_1"] != "http.sslbackend" || vars["GIT_CONFIG_COUNT"] != "2" {
		t.Errorf("gh's git gets %v", vars)
	}
}
