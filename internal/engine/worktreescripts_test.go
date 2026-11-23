package engine

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// SES-016: a new worktree gets the ignored files it names and its setup
// before its first prompt runs; each worktree session has its own ports; a
// failed setup holds the session until the user retries or skips it; the
// archive command runs before the worktree goes.
func TestWorktreeScripts(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\nsetup.txt\nport.txt\n"), 0o644)
	gitIn(t, repo, "add", ".gitignore")
	gitIn(t, repo, "commit", "-qm", "ignore")
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte(
		"copy: [.env, missing.json]\nsetup: sleep 0.3; echo set up > setup.txt; echo $FREELOOP_PORT > port.txt\narchive: touch \"$FREELOOP_ROOT_PATH/archived.txt\"\n"), 0o644)

	e := openTest(t, t.TempDir())
	defer e.Close()
	ch, cancel := e.Subscribe()
	defer cancel()
	p := scripted.New("sp", scripted.Step{Text: "hello"}, scripted.Step{Text: "again"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	if info, _ := e.ProjectSettings(ctx, repo); info.Worktree.Setup == "" {
		t.Error("the trust prompt does not show the setup command")
	}
	e.SetProjectTrust(ctx, repo, true)

	a, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.PortBase < FirstPort || b.PortBase == a.PortBase {
		t.Errorf("ports = %d, %d", a.PortBase, b.PortBase)
	}
	// The prompt starts only once the setup has written its file.
	if _, err := e.Prompt(ctx, a.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, a.ID)
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, "setup.txt")); string(got) != "set up\n" {
		t.Errorf("setup.txt = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, "port.txt")); strings.TrimSpace(string(got)) != strconv.Itoa(a.PortBase) {
		t.Errorf("port.txt = %q, want %d", got, a.PortBase)
	}
	if got, _ := os.ReadFile(filepath.Join(a.Worktree, ".env")); string(got) != "SECRET=1\n" {
		t.Errorf(".env = %q", got)
	}
	evs, _ := e.Events(ctx, a.ID, 0, 0)
	var done string
	for _, ev := range evs {
		if strings.Contains(string(ev.Data), `"status":"done"`) && ev.Type == "worktree_setup" {
			done = string(ev.Data)
		}
	}
	if !strings.Contains(done, `"copied":[".env"]`) || !strings.Contains(done, `"missing":["missing.json"]`) {
		t.Errorf("setup event = %s", done)
	}

	// A failing setup holds the session until it is skipped.
	if err := e.SetProjectScripts(ctx, a.ProjectID, config.WorktreeScripts{Setup: "echo broken; exit 3"}); err != nil {
		t.Fatal(err)
	}
	c, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "sp", Model: "m", Mode: "ask", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, ch, func(ev event.Event) bool {
		return ev.SID == c.ID && ev.Type == "worktree_setup" && strings.Contains(string(ev.Data), "failed")
	})
	if _, err := e.Prompt(ctx, c.ID, "hi"); err == nil || !strings.Contains(err.Error(), "worktree setup failed") {
		t.Errorf("prompt after a failed setup = %v", err)
	}
	e.SkipSetup(ctx, c.ID)
	if _, err := e.Prompt(ctx, c.ID, "hi"); err != nil {
		t.Errorf("prompt after skipping = %v", err)
	}
	waitIdle(t, ch, c.ID)
	info, _ := e.ProjectScripts(ctx, a.ProjectID)
	if info.Effective.Setup != "echo broken; exit 3" || info.Effective.Run != "" || len(info.Effective.Copy) != 2 {
		t.Errorf("effective scripts = %+v", info.Effective)
	}

	// Archiving runs the archive command.
	if err := e.ArchiveSession(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(repo, "archived.txt")); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the archive command did not run")
}

// standInDocker puts a docker on PATH that records its calls in the returned
// file and keeps one container's state.
func standInDocker(t *testing.T) (calls string) {
	t.Helper()
	dir := t.TempDir()
	calls, state := filepath.Join(dir, "calls"), filepath.Join(dir, "state")
	os.WriteFile(filepath.Join(dir, "docker"), []byte(`#!/bin/sh
echo "$*" >> '`+calls+`'
case "$1" in
inspect) cat '`+state+`' 2>/dev/null || exit 1 ;;
run|start) echo running > '`+state+`' ;;
stop) echo exited > '`+state+`' ;;
exec) case "$*" in *" ls -A "*) echo a.txt ;; esac ;;
esac
`), 0o755)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

// OQ-026: in a Docker environment the setup and archive commands run in the
// session's container, never on the Mac, where they could run what the
// container wrote into the worktree; it stops again when no session uses it.
// The copy still comes from the main checkout, on the host.
func TestScriptsRunInTheContainer(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the stand-in docker is a shell script, and Windows runs one only through a shell; the app runs docker directly")
	}
	ctx := context.Background()
	calls := standInDocker(t)
	repo := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte(
		"copy: [.env]\nsetup: touch "+shPath(marker)+"-setup\narchive: touch "+shPath(marker)+"-archive\n"), 0o644)
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	e.SetProjectTrust(ctx, repo, true)
	os.MkdirAll(filepath.Join(dir, config.EnvironmentsDir), 0o700)
	os.WriteFile(filepath.Join(dir, config.EnvironmentsDir, "box.yaml"), []byte("name: Box\nkind: docker\nimage: alpine:3.20\n"), 0o600)
	e.loadEnvironments()
	approveFiles(t, e)

	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID, EnvironmentID: "box", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.waitSetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	log := func() string {
		b, _ := os.ReadFile(calls)
		return string(b)
	}
	ranIn := func(what string) {
		t.Helper()
		for _, line := range strings.Split(log(), "\n") {
			if strings.HasPrefix(line, "exec ") && strings.Contains(line, shPath(marker)+"-"+what) {
				return
			}
		}
		t.Errorf("the %s command did not run in the container:\n%s", what, log())
	}
	ranIn("setup")
	if !strings.HasSuffix(strings.TrimSpace(log()), containerName(ses.ID)) || !strings.Contains(log(), "stop ") {
		t.Errorf("the container was left running:\n%s", log())
	}
	if b, err := os.ReadFile(filepath.Join(ses.Worktree, ".env")); err != nil || string(b) != "SECRET=1\n" {
		t.Errorf("the copied file: %q, %v", b, err)
	}

	// A retry after the container has run stays in the container too.
	os.Remove(calls)
	if err := e.RetrySetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.waitSetup(ctx, ses.ID); err != nil {
		t.Fatal(err)
	}
	ranIn("setup")

	os.Remove(calls)
	if err := e.ArchiveSession(ctx, ses.ID, true); err != nil {
		t.Fatal(err)
	}
	ranIn("archive")
	for _, what := range []string{"setup", "archive"} {
		if _, err := os.Stat(marker + "-" + what); err == nil {
			t.Errorf("the %s command ran on the Mac", what)
		}
	}

	// A container that never started has nothing to clean up.
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte("archive: touch "+shPath(marker)+"-archive\n"), 0o644)
	approveWorktreeScripts(t, e)
	other, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID, EnvironmentID: "box", Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(calls)
	e.runArchiveScript(ctx, other)
	if strings.Contains(log(), "exec ") || strings.Contains(log(), "run ") {
		t.Errorf("a container started for nothing:\n%s", log())
	}

	// A worktree a container has had does not go back to the Mac.
	moved := other
	moved.EnvironmentID = ""
	e.emit(other.ID, event.EnvHealth, map[string]string{"status": "healthy", "container": containerName(other.ID)})
	if _, _, err := e.scriptHost(ctx, moved); err == nil {
		t.Error("a worktree a container had would run its commands on the Mac")
	}
}

// worktreeScriptsWaiting lists the trusted projects' worktree scripts waiting for approval.
func worktreeScriptsWaiting(e *Engine) []PendingCommand {
	return slices.DeleteFunc(e.PendingCommands(), func(c PendingCommand) bool { return c.Kind != pendingProjectWorktree })
}

// approveWorktreeScripts approves the worktree scripts a test changed after trusting.
func approveWorktreeScripts(t *testing.T, e *Engine) {
	t.Helper()
	for _, c := range worktreeScriptsWaiting(e) {
		if _, err := e.ApproveCommand(c.Fingerprint); err != nil {
			t.Fatal(err)
		}
	}
}

// SEC-003, SEC-006, SES-016 (issue 064): trusting a project approves its worktree
// scripts as they are, its copy list included, as it does its hooks. Scripts changed
// since, by a pull or a branch switch, run none of their commands and copy nothing; they
// wait in Settings and the session says why, until approved as they are now.
// Distrusting ends the approval.
func TestChangedWorktreeScriptsWaitForApproval(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	marks := t.TempDir()
	ran := func(what string) bool {
		_, err := os.Stat(filepath.Join(marks, what))
		return err == nil
	}
	write := func(name string) {
		t.Helper()
		os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
		if err := os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte("copy: [.env]\n"+
			"setup: touch "+shPath(filepath.Join(marks, "setup-"+name))+"\n"+
			"run: echo run "+name+"\n"+
			"archive: touch "+shPath(filepath.Join(marks, "archive-"+name))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := openTest(t, t.TempDir())
	defer e.Close()
	create := func() session.Session {
		t.Helper()
		ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID, Worktree: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.waitSetup(ctx, ses.ID); err != nil {
			t.Fatal(err)
		}
		return ses
	}
	copied := func(ses session.Session) bool {
		_, err := os.Stat(filepath.Join(ses.Worktree, ".env"))
		return err == nil
	}

	write("A")
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	if w := worktreeScriptsWaiting(e); len(w) != 0 {
		t.Fatalf("trusting left the scripts waiting: %+v", w)
	}
	a := create()
	if !ran("setup-A") || !copied(a) {
		t.Fatalf("the approved scripts: setup ran %v, .env copied %v", ran("setup-A"), copied(a))
	}

	// A pull changes them: nothing of the file runs or is copied, and Settings and the
	// session say why.
	write("B")
	list := worktreeScriptsWaiting(e)
	if len(list) != 1 || !strings.Contains(list[0].Runs, "setup-B") || !strings.Contains(list[0].Runs, ".env") || !strings.Contains(list[0].Source, filepath.Join(environment.Canonical(repo), ProjectDir, config.WorktreeFileName)) {
		t.Fatalf("waiting = %+v", list)
	}
	if !slices.ContainsFunc(e.ConfigIssues(), func(i config.Issue) bool { return i.Approval && strings.Contains(i.Message, "setup-B") }) {
		t.Error("the problems list does not name the waiting scripts")
	}
	b := create()
	if ran("setup-B") || copied(b) {
		t.Errorf("changed scripts ran unapproved: setup %v, .env copied %v", ran("setup-B"), copied(b))
	}
	evs, _ := e.Events(ctx, b.ID, 0, 0)
	if !slices.ContainsFunc(evs, func(ev event.Event) bool {
		return ev.Type == event.Notice && strings.Contains(string(ev.Data), "not the ones approved") && strings.Contains(string(ev.Data), "setup-B")
	}) {
		t.Error("the session does not say the project's worktree scripts wait for approval")
	}
	if info, _ := e.ProjectScripts(ctx, b.ProjectID); !info.RepoWaiting || info.Effective.Setup != "" || info.Effective.Run != "" || len(info.Effective.Copy) != 0 {
		t.Errorf("scripts while waiting = %+v", info)
	}
	if _, err := e.RunWorktreeScript(ctx, b.ID); err == nil || !strings.Contains(err.Error(), "Settings") {
		t.Errorf("the run command while waiting: %v", err)
	}
	e.runArchiveScript(ctx, a)
	if ran("archive-A") || ran("archive-B") {
		t.Error("an archive command ran unapproved")
	}
	// What the user set in the app is their own and still applies.
	if err := e.SetProjectScripts(ctx, b.ProjectID, config.WorktreeScripts{Archive: "touch " + shPath(filepath.Join(marks, "archive-app"))}); err != nil {
		t.Fatal(err)
	}
	e.runArchiveScript(ctx, a)
	if !ran("archive-app") || ran("archive-B") {
		t.Errorf("the app's own archive command ran %v, the file's %v", ran("archive-app"), ran("archive-B"))
	}
	e.SetProjectScripts(ctx, b.ProjectID, config.WorktreeScripts{})

	// Approved as they are now, they run.
	if _, err := e.ApproveCommand(list[0].Fingerprint); err != nil {
		t.Fatal(err)
	}
	if w := worktreeScriptsWaiting(e); len(w) != 0 {
		t.Fatalf("still waiting after approval: %+v", w)
	}
	c := create()
	if !ran("setup-B") || !copied(c) {
		t.Errorf("approved scripts: setup ran %v, .env copied %v", ran("setup-B"), copied(c))
	}
	e.runArchiveScript(ctx, a)
	if !ran("archive-B") {
		t.Error("the approved archive command did not run")
	}

	// Distrusting ends the approval; trusting again approves what is there then.
	if _, err := e.SetProjectTrust(ctx, repo, false); err != nil {
		t.Fatal(err)
	}
	write("C")
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	write("B")
	if w := worktreeScriptsWaiting(e); len(w) != 1 {
		t.Errorf("B after distrust: waiting %+v", w)
	}
}

// Issue 064: a project trusted before its worktree scripts needed approval is asked
// once: they wait, are not approved by a new start, and once approved stay so.
func TestWorktreeScriptsOfAnEarlierTrustAreAskedOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	mem := secrets.NewMemory()
	e := openFor(t, dir, mem)
	defer func() { e.Close() }()
	repo := gitRepo(t)
	os.MkdirAll(filepath.Join(repo, ProjectDir), 0o755)
	os.WriteFile(filepath.Join(repo, ProjectDir, config.WorktreeFileName), []byte("setup: npm install\n"), 0o644)
	// Trusted by a version that approved no worktree scripts.
	p, err := e.AddProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	p.Trusted = true
	if err := e.store.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	reopen := func() {
		e.Close()
		e = openFor(t, dir, mem)
	}
	reopen()
	list := worktreeScriptsWaiting(e)
	if len(list) != 1 || !strings.Contains(list[0].Runs, "npm install") {
		t.Fatalf("waiting = %+v", list)
	}
	reopen()
	if len(worktreeScriptsWaiting(e)) != 1 {
		t.Fatal("a new start approved them")
	}
	if _, err := e.ApproveCommand(list[0].Fingerprint); err != nil {
		t.Fatal(err)
	}
	reopen()
	if w := worktreeScriptsWaiting(e); len(w) != 0 {
		t.Errorf("asked again: %+v", w)
	}
}
