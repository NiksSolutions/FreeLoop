package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func fileState(c Changes, path string) (ChangedFile, bool) {
	for _, f := range c.Files {
		if f.Path == path {
			return f, true
		}
	}
	return ChangedFile{}, false
}

// GUI-005: changes against the base commit, per-file diffs, stage, commit
// with a message that lands in git log, and discard.
func TestDiffPanel(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	gitIn(t, repo, "config", "user.name", "t")
	gitIn(t, repo, "config", "user.email", "t@t")
	e := openTest(t, t.TempDir())
	defer e.Close()
	withScript(e, "msg", scripted.Step{Text: "Change a and add b"})
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: "msg", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	if ses.BaseCommit == "" {
		t.Fatal("no base commit")
	}

	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "c.txt"), []byte("c\n"), 0o644)
	gitIn(t, repo, "add", "c.txt")
	gitIn(t, repo, "commit", "-q", "-m", "c after the base")

	c, err := e.Changes(ctx, ses.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Repo || c.Branch != "main" || c.Base != ses.BaseCommit || len(c.Files) != 3 {
		t.Fatalf("changes = %+v", c)
	}
	if f, _ := fileState(c, "a.txt"); f.Status != "modified" || f.Staged || !f.Unstaged {
		t.Errorf("a.txt = %+v", f)
	}
	if f, _ := fileState(c, "b.txt"); f.Status != "untracked" || !f.Unstaged {
		t.Errorf("b.txt = %+v", f)
	}
	if f, _ := fileState(c, "c.txt"); f.Status != "added" || f.Staged || f.Unstaged {
		t.Errorf("c.txt (committed after the base) = %+v", f)
	}

	d, err := e.FileDiff(ctx, ses.ID, "a.txt")
	if err != nil || !strings.Contains(d.Diff, "-a") || !strings.Contains(d.Diff, "+changed") {
		t.Errorf("diff a.txt = %q, %v", d.Diff, err)
	}
	if d, err = e.FileDiff(ctx, ses.ID, "b.txt"); err != nil || !strings.Contains(d.Diff, "+b") {
		t.Errorf("diff b.txt = %q, %v", d.Diff, err)
	}
	for _, p := range []string{"../outside", "/etc/passwd", "nope.txt"} {
		if _, err := e.FileDiff(ctx, ses.ID, p); err == nil {
			t.Errorf("diff of %s succeeded", p)
		}
		if err := e.StageFiles(ctx, ses.ID, []string{p}, true); err == nil {
			t.Errorf("staging %s succeeded", p)
		}
	}

	if err := e.StageFiles(ctx, ses.ID, []string{"a.txt", "b.txt"}, true); err != nil {
		t.Fatal(err)
	}
	if err := e.StageFiles(ctx, ses.ID, []string{"b.txt"}, false); err != nil {
		t.Fatal(err)
	}
	c, _ = e.Changes(ctx, ses.ID)
	if f, _ := fileState(c, "a.txt"); !f.Staged || f.Unstaged {
		t.Errorf("a.txt after staging = %+v", f)
	}
	if f, _ := fileState(c, "b.txt"); f.Status != "untracked" {
		t.Errorf("b.txt after unstaging = %+v", f)
	}

	msg, err := e.SuggestCommitMessage(ctx, ses.ID)
	if err != nil || msg != "Change a and add b" {
		t.Errorf("suggestion = %q, %v", msg, err)
	}
	if _, err := e.Commit(ctx, ses.ID, "  "); err == nil {
		t.Error("empty message committed")
	}
	head, err := e.Commit(ctx, ses.ID, "Change a from the panel")
	if err != nil {
		t.Fatal(err)
	}
	if got := gitIn(t, repo, "log", "-1", "--format=%H %s"); got != head+" Change a from the panel" {
		t.Errorf("git log = %q, head %s", got, head)
	}
	c, _ = e.Changes(ctx, ses.ID)
	if f, _ := fileState(c, "a.txt"); f.Status != "modified" || f.Staged || f.Unstaged {
		t.Errorf("a.txt after commit = %+v", f)
	}

	// Discard: a tracked file goes back to HEAD, an untracked one is
	// deleted, a committed change is refused.
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("again\n"), 0o644)
	if err := e.DiscardFile(ctx, ses.ID, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "a.txt")); string(b) != "changed\n" {
		t.Errorf("a.txt after discard = %q", b)
	}
	if err := e.DiscardFile(ctx, ses.ID, "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); !os.IsNotExist(err) {
		t.Errorf("b.txt still there: %v", err)
	}
	if err := e.DiscardFile(ctx, ses.ID, "c.txt"); err == nil {
		t.Error("discarding a committed change succeeded")
	}
}

// A repository whose own config names programs git would run is off limits
// for the panel until projects are trusted (SEC-006).
func TestDiffPanelRefusesCommandConfig(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.SetProjectTrust(ctx, repo, true)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	// Slashes, so the filter's shell (Git Bash on Windows) does not read the
	// backslashes as escapes and touch somewhere the check does not look.
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "ran"))
	gitIn(t, repo, "config", "filter.evil.clean", "touch "+shPath(marker))
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("* filter=evil\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed"), 0o644)

	c, err := e.Changes(ctx, ses.ID)
	if err != nil || c.Blocked == "" || len(c.Files) != 0 {
		t.Errorf("changes = %+v, %v; want blocked", c, err)
	}
	if err := e.StageFiles(ctx, ses.ID, []string{"a.txt"}, true); err == nil {
		t.Error("staging succeeded")
	}
	if _, err := e.Commit(ctx, ses.ID, "x"); err == nil {
		t.Error("commit succeeded")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the filter ran")
	}
}

// A filter set in the per-worktree config.worktree, as a command in a
// container could write it through the bind-mounted .git, is found too:
// neither the git state nor the diff panel runs it on the host.
func TestWorktreeConfigFilterIsRefused(t *testing.T) {
	ctx := context.Background()
	repo := gitRepo(t)
	e := openTest(t, t.TempDir())
	defer e.Close()
	e.SetProjectTrust(ctx, repo, true)
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	// Slashes: in a config file a backslash starts an escape, and C:\Users is a
	// line git refuses, so on Windows no git command would run at all; and the
	// filter's shell, Git Bash there, reads C:/Users as the folder it is.
	marker := filepath.ToSlash(filepath.Join(t.TempDir(), "ran"))
	gitIn(t, repo, "config", "core.repositoryformatversion", "1")
	gitIn(t, repo, "config", "extensions.worktreeConfig", "true")
	os.WriteFile(filepath.Join(repo, ".git", "config.worktree"), []byte("[filter \"evil\"]\n\tclean = touch "+shPath(marker)+"\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("* filter=evil\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed"), 0o644)

	env, _ := e.localEnv(repo, nil)
	if st := gitState(ctx, env, true); st.Dirty != -1 {
		t.Errorf("git state = %+v; want the file scan skipped", st)
	}
	if c, err := e.Changes(ctx, ses.ID); err != nil || c.Blocked == "" {
		t.Errorf("changes = %+v, %v; want blocked", c, err)
	}
	if err := e.StageFiles(ctx, ses.ID, []string{"a.txt"}, true); err == nil {
		t.Error("staging succeeded")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the filter ran")
	}
}

// A file's name is never read as a git pattern: discarding, staging or unstaging a
// file an agent named src/*.bak, :^nothing or :!zz touches that file and no other.
func TestFileNamesAreNotGitPatterns(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Windows file names cannot hold * or :")
	}
	ctx := context.Background()
	repo := gitRepo(t)
	write := func(name, text string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o755)
		if err := os.WriteFile(filepath.Join(repo, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(repo, name))
		return string(b)
	}
	write("zz", "zz\n")
	write(":!zz", "committed\n")
	gitIn(t, repo, "add", "--", "zz", ":(literal):!zz")
	gitIn(t, repo, "commit", "-q", "-m", "zz")
	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.SetProjectTrust(ctx, repo, true); err != nil {
		t.Fatal(err)
	}
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: repo, ProviderID: DemoProviderID})
	if err != nil {
		t.Fatal(err)
	}
	write("src/*.bak", "x")
	write("src/u1.bak", "u1")
	write("src/u2.bak", "u2")
	write(":^nothing", "x")
	write("a.txt", "changed")
	write("zz", "changed")
	write(":!zz", "changed")

	if err := e.DiscardFile(ctx, ses.ID, "src/*.bak"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "src/*.bak")); !os.IsNotExist(err) {
		t.Errorf("src/*.bak is still there: %v", err)
	}
	if read("src/u1.bak") != "u1" || read("src/u2.bak") != "u2" {
		t.Error("discarding src/*.bak deleted the other .bak files")
	}
	if err := e.DiscardFile(ctx, ses.ID, ":^nothing"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ":^nothing")); !os.IsNotExist(err) {
		t.Errorf(":^nothing is still there: %v", err)
	}
	if read("src/u1.bak") != "u1" || read("src/u2.bak") != "u2" {
		t.Error("discarding :^nothing deleted the other untracked files")
	}
	if err := e.DiscardFile(ctx, ses.ID, ":!zz"); err != nil {
		t.Fatal(err)
	}
	if got := read(":!zz"); got != "committed\n" {
		t.Errorf(":!zz after discard = %q", got)
	}
	if read("a.txt") != "changed" || read("zz") != "changed" {
		t.Error("discarding :!zz threw away the changes of other files")
	}

	staged := func(name string) bool {
		t.Helper()
		c, err := e.Changes(ctx, ses.ID)
		if err != nil {
			t.Fatal(err)
		}
		f, _ := fileState(c, name)
		return f.Staged
	}
	write("src/*.bak", "x")
	write("src/u1.bak", "u1")
	write("src/u2.bak", "u2")
	write(":^nothing", "x")
	if err := e.StageFiles(ctx, ses.ID, []string{"src/*.bak"}, true); err != nil {
		t.Fatal(err)
	}
	if !staged("src/*.bak") || staged("src/u1.bak") || staged("src/u2.bak") {
		t.Error("staging src/*.bak staged other files")
	}
	if err := e.StageFiles(ctx, ses.ID, []string{":^nothing"}, true); err != nil {
		t.Fatal(err)
	}
	if !staged(":^nothing") || staged("a.txt") || staged("zz") || staged("src/u1.bak") {
		t.Error("staging :^nothing staged other files")
	}
	if err := e.StageFiles(ctx, ses.ID, []string{"src/u1.bak"}, true); err != nil {
		t.Fatal(err)
	}
	if err := e.StageFiles(ctx, ses.ID, []string{"src/*.bak"}, false); err != nil {
		t.Fatal(err)
	}
	if staged("src/*.bak") || !staged("src/u1.bak") || !staged(":^nothing") {
		t.Error("unstaging src/*.bak unstaged other files")
	}
	if err := e.StageFiles(ctx, ses.ID, []string{":^nothing"}, false); err != nil {
		t.Fatal(err)
	}
	if staged(":^nothing") || !staged("src/u1.bak") {
		t.Error("unstaging :^nothing unstaged other files")
	}
}
