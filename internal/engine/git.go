package engine

import (
	"context"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// The app's own git queries run through the session's environment like
// every process, but outside the permission broker: they are reads the app
// makes for its own display, not side effects of a harness.

// A repository's own .git/config can make git run commands
// (core.fsmonitor, hooks, content filters). The app's git calls turn off
// what can be turned off; in a project the user has not trusted (SEC-006)
// they are limited to plumbing reads that run nothing (rev-parse,
// symbolic-ref, rev-list, config --get), and in a trusted one the file scan
// is still skipped where filters are configured. Asking a nested repository
// whether its files changed starts git in it, under a config this repository
// knows nothing of; only the commit it stands at is compared.
//
// Even plumbing can run a program if it is allowed to fetch: a missing object in
// a repository with a promisor remote makes rev-list or status fetch it, and an
// ext:: URL is a command. So nothing here may fetch, whatever the config says.
// Only the push, which has to, gets its own list of transports.
var safeGit = []string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.ignoreSubmodules=dirty"}

// gitArgv is safeGit followed by args, for git in env. On Windows /dev/null is no
// file to Git for Windows' hook lookup but \dev\null on the current drive, a
// folder anyone may make; hooks are looked for in System32 instead, which only an
// administrator can write (NFR-XPLAT-003).
func gitArgv(env environment.Environment, args ...string) []string {
	base := safeGit
	if localWindows(env) {
		base = safeGitOn("windows", os.Getenv("SystemRoot"))
	}
	return append(slices.Clone(base), args...)
}

// safeGitOn is safeGit for a git running on goos.
func safeGitOn(goos, systemRoot string) []string {
	out := slices.Clone(safeGit)
	if goos != "windows" {
		return out
	}
	if systemRoot == "" || !strings.Contains(systemRoot, `:\`) {
		systemRoot = `C:\Windows`
	}
	for i, a := range out {
		if a == "core.hooksPath=/dev/null" {
			out[i] = "core.hooksPath=" + strings.TrimRight(systemRoot, `\`) + `\System32\freeloop-no-hooks`
		}
	}
	return out
}

// gitEnv is the environment of the app's git commands, with more added. Git reads
// the paths it is given literally (GIT_LITERAL_PATHSPECS): otherwise a file an agent
// named src/*.bak or :^x stands for other files too, and discarding it deletes them.
// A call that needs git's patterns adds gitPatterns and says why.
func gitEnv(more map[string]string) map[string]string {
	out := map[string]string{"GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_NO_LAZY_FETCH": "1", "GIT_ALLOW_PROTOCOL": "", "GIT_LITERAL_PATHSPECS": "1"}
	maps.Copy(out, more)
	return out
}

// gitPatterns, added to gitEnv, has git read pathspec magic and wildcards again. A
// call with it names only the app's own paths, or keeps a name from being read as a
// pattern some other way.
var gitPatterns = map[string]string{"GIT_LITERAL_PATHSPECS": "0"}

// gitEnvFor is gitEnv for git in env.
func gitEnvFor(env environment.Environment, more map[string]string) map[string]string {
	if localWindows(env) {
		return gitEnvOn("windows", systemGitOf(env), more)
	}
	return gitEnv(more)
}

// gitEnvOn is gitEnv for a git running on goos, given what its system config gives it
// (systemGit). Git for Windows keeps in its system config, under Program Files where
// only an administrator writes, what makes it work there: core.autocrlf, without which
// every file checked out with \r\n shows as changed, and the credential manager a push
// signs in with (NFR-XPLAT-003). With (b) git still reads no system config and gets
// those two on its command line (systemGitArgs); with (a) it reads the system config.
func gitEnvOn(goos string, sys systemGit, more map[string]string) map[string]string {
	out := gitEnv(more)
	if goos == "windows" && !sys.Narrow {
		delete(out, "GIT_CONFIG_NOSYSTEM")
	}
	return out
}

// gitArgvFor is gitArgv for a command of the app's own in env, with what git's system
// config gives a local git on Windows (systemGitArgs) before args, so that the
// command's own settings win.
func gitArgvFor(ctx context.Context, env environment.Environment, args ...string) []string {
	if localWindows(env) {
		args = append(systemGitArgs(ctx, env, systemGitOf(env), args), args...)
	}
	return gitArgv(env, args...)
}

// systemGit is what Git for Windows' system config gives the app's git (OQ-046, decided
// 2026-11-25). Narrow is (b): git reads no system config and gets core.autocrlf and the
// credential helpers, Git Credential Manager by its own path, on its command line, so no
// other program the file names runs. Otherwise it is (a): git reads the system config,
// which only an administrator can change, with hooks off (safeGitOn).
type systemGit struct {
	Narrow   bool
	AutoCRLF string   // "" where the system config does not set it
	Helpers  []string // credential.helper's values, as configured
	// TLS is the system's http.sslbackend and http.sslcainfo, which Git for Windows
	// sets so its curl finds a CA bundle: a name and a file, not a program. Without
	// them an https push can fail on the certificate.
	TLS [][2]string
}

// systemGitQuery asks git for the system settings systemGitFrom decides on. It exits 1
// when none is set, as with no system config.
var systemGitQuery = []string{"config", "--system", "--includes", "-z", "--get-regexp", `^(core\.autocrlf|http\.sslbackend|http\.sslcainfo|credential\.(.*\.)?helper)$`}

// sslBackends are the values Git for Windows reads for http.sslbackend.
var sslBackends = []string{"openssl", "schannel"}

// autoCRLFValues are the values git reads for core.autocrlf, in any case.
var autoCRLFValues = []string{"true", "false", "input", "yes", "no", "on", "off", "1", "0"}

// systemGitFrom decides from what systemGitQuery printed: (b) when the credential
// helpers left after the last empty one, which clears those before it, are all Git
// Credential Manager by its own path (gcmHelper); (a) for any other helper, a helper
// for one address, or none at all, as with no system config. The empty one is not
// passed on: on the command line it would clear the user's own helpers too, which git
// reads after the system's.
func systemGitFrom(out string) systemGit {
	var helpers []string
	autocrlf, other := "", false
	backend, bundle := "", ""
	for _, entry := range strings.Split(out, "\x00") {
		key, value, hasValue := strings.Cut(entry, "\n")
		switch {
		case entry == "":
		case key == "http.sslbackend":
			backend = ""
			if slices.Contains(sslBackends, strings.ToLower(value)) {
				backend = strings.ToLower(value)
			}
		case key == "http.sslcainfo":
			bundle = value
		case key == "core.autocrlf":
			if !hasValue {
				value = "true" // a key with no value is true to git
			}
			autocrlf = ""
			if slices.Contains(autoCRLFValues, strings.ToLower(value)) {
				autocrlf = value
			}
		case key == "credential.helper" && hasValue && value == "":
			helpers = nil
		case key == "credential.helper" && hasValue:
			helpers = append(helpers, value)
		default: // a helper for one address, or one with no value, which git refuses
			other = true
		}
	}
	if other || len(helpers) == 0 || slices.ContainsFunc(helpers, func(h string) bool { return !gcmHelper(h) }) {
		return systemGit{}
	}
	var tls [][2]string
	if backend != "" {
		tls = append(tls, [2]string{"http.sslbackend", backend})
	}
	if bundle != "" {
		tls = append(tls, [2]string{"http.sslcainfo", bundle})
	}
	return systemGit{Narrow: true, AutoCRLF: autocrlf, Helpers: helpers, TLS: tls}
}

// gcmNames are Git Credential Manager's file names, under its name and its older one,
// GCM Core.
var gcmNames = []string{"git-credential-manager.exe", "git-credential-manager", "git-credential-manager-core.exe", "git-credential-manager-core"}

// gcmHelper reports a credential helper that is Git Credential Manager by its own path:
// manager, which Git for Windows writes and git runs as git-credential-manager from its
// own folders, manager-core, its older name, or the absolute path of that program with
// nothing after it. Git runs a helper through the shell, so such a path spells folders
// with / and escapes a space or a bracket with \, and holds nothing else the shell reads.
func gcmHelper(h string) bool {
	if h == "manager" || h == "manager-core" {
		return true
	}
	if len(h) < 3 || !isLetter(h[0]) || h[1] != ':' || h[2] != '/' {
		return false
	}
	var path strings.Builder
	path.WriteString(h[:3])
	for i := 3; i < len(h); i++ {
		c := h[i]
		switch {
		case c == '\\' && i+1 < len(h) && strings.IndexByte(" ()", h[i+1]) >= 0:
			i++
			path.WriteByte(h[i])
		case isLetter(c) || c >= '0' && c <= '9' || strings.IndexByte("/.-_", c) >= 0:
			path.WriteByte(c)
		default:
			return false
		}
	}
	p := path.String()
	name := p[strings.LastIndexByte(p, '/')+1:]
	return slices.ContainsFunc(gcmNames, func(n string) bool { return strings.EqualFold(n, name) })
}

// systemGitRead is what git's system config gives, read once a run: the file needs an
// administrator to change, and reading it for each call would double them.
var systemGitRead struct {
	sync.Mutex
	done bool
	sys  systemGit
}

// systemGitOf is what git's system config gives git in env, a local environment on
// Windows. A git that did not run, not installed or too slow, is asked again next time
// and gives (a) meanwhile, which passes nothing.
func systemGitOf(env environment.Environment) systemGit {
	systemGitRead.Lock()
	defer systemGitRead.Unlock()
	if !systemGitRead.done {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := env.Exec(ctx, environment.ExecSpec{
			Argv:           gitArgv(env, systemGitQuery...),
			Env:            gitEnv(nil),
			Cwd:            neutralDirFor(env),
			SeparateStderr: true,
			MaxOutput:      1 << 16,
		})
		if err == nil && !res.TimedOut {
			systemGitRead.done = true
			if res.ExitCode == 0 { // 1: nothing set, or no system config
				systemGitRead.sys = systemGitFrom(res.Output)
			}
		}
	}
	return systemGitRead.sys
}

// systemGitArgs is what sys gives a git command of args in env, as -c options
// (systemGitPairs).
func systemGitArgs(ctx context.Context, env environment.Environment, sys systemGit, args []string) []string {
	opts, command := gitCommandOf(args)
	var out []string
	for _, kv := range systemGitPairs(ctx, env, sys, opts, command != "" && !slices.Contains(convertsNothing, command), slices.Contains(talksToRemotes, command)) {
		out = append(out, "-c", kv[0]+"="+kv[1])
	}
	return out
}

// systemGitPairs is what sys gives a git command with git's own options opts, as keys
// and values: the credential helpers; for a command that talks to a remote, the TLS
// settings the user's own config does not set; and, for a command that converts line
// endings, core.autocrlf, unless opts or the user's own config set it. Passed on the
// command line a value would win over the user's, while git reads the system's below
// them; so git is asked first, without the system's file.
func systemGitPairs(ctx context.Context, env environment.Environment, sys systemGit, opts []string, converts, network bool) [][2]string {
	if !sys.Narrow {
		return nil
	}
	var out [][2]string
	for _, h := range sys.Helpers {
		out = append(out, [2]string{"credential.helper", h})
	}
	if network && len(sys.TLS) > 0 {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, err := env.Exec(ctx, environment.ExecSpec{
			Argv: gitArgv(env, append(slices.Clone(opts), "config", "--name-only", "--get-regexp", `^http\.(sslbackend|sslcainfo)$`)...),
			Env:  gitEnv(nil),
		})
		cancel()
		// Exit 1 is nothing set; anything else unread passes nothing, as a guess
		// could put the system's bundle above the user's.
		if err == nil && !res.TimedOut && (res.ExitCode == 0 || res.ExitCode == 1) {
			set := strings.Fields(strings.ToLower(res.Output))
			for _, kv := range sys.TLS {
				if !slices.Contains(set, kv[0]) {
					out = append(out, kv)
				}
			}
		}
	}
	if sys.AutoCRLF == "" || !converts || slices.ContainsFunc(opts, func(o string) bool { return strings.HasPrefix(strings.ToLower(o), "core.autocrlf=") }) {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv: gitArgv(env, append(slices.Clone(opts), "config", "--get", "core.autocrlf")...),
		Env:  gitEnv(nil),
	})
	if err == nil && !res.TimedOut && res.ExitCode == 1 {
		out = append(out, [2]string{"core.autocrlf", sys.AutoCRLF})
	}
	return out
}

// talksToRemotes are the git commands, as the app runs them, that reach a remote, and so
// need the system's TLS settings.
var talksToRemotes = []string{"push", "fetch", "ls-remote", "pull", "clone"}

// convertsNothing are the git commands, as the app runs them, that never move a file
// between the working tree and the repository, so core.autocrlf means nothing to them
// and git is not asked about it. A command not listed is asked, which costs a git config.
var convertsNothing = []string{"config", "rev-parse", "symbolic-ref", "rev-list", "log", "merge-base", "remote", "branch", "cat-file", "check-ignore", "diff-tree", "ls-tree", "init", "push", "fetch", "ls-remote", "for-each-ref", "show-ref"}

// gitCommandOf splits what follows "git" into git's own options and the command they
// come before.
func gitCommandOf(args []string) (opts []string, command string) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case !strings.HasPrefix(a, "-"):
			return args[:i:i], a
		case slices.Contains([]string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env"}, a):
			i++ // its value
		}
	}
	return args, ""
}

func git(ctx context.Context, env environment.Environment, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv:      gitArgvFor(ctx, env, args...),
		Env:       gitEnvFor(env, nil),
		MaxOutput: 1 << 20,
	})
	if err != nil || res.ExitCode != 0 || res.TimedOut {
		return "", false
	}
	return res.Output, true
}

// gitState reads branch, HEAD and the number of changed files. In an untrusted
// project, or a repository whose config names programs or whose common directory
// was moved, both of which git status may act on, it reads only branch and HEAD
// and reports the count as unknown (-1).
func gitState(ctx context.Context, env environment.Environment, trusted bool) event.GitStateData {
	if !trusted || blocked(ctx, env) != "" {
		if _, ok := git(ctx, env, "rev-parse", "--git-dir"); !ok {
			return event.GitStateData{}
		}
		st := event.GitStateData{Repo: true, Dirty: -1}
		if out, ok := git(ctx, env, "rev-parse", "--verify", "--quiet", "HEAD"); ok {
			st.Head = strings.TrimSpace(out)
		}
		if out, ok := git(ctx, env, "symbolic-ref", "--quiet", "--short", "HEAD"); ok {
			st.Branch = strings.TrimSpace(out)
		}
		return st
	}
	out, ok := git(ctx, env, "status", "--porcelain=v2", "--branch", "--ignore-submodules=all")
	if !ok {
		return event.GitStateData{}
	}
	return parseStatus(out)
}

// repoConfigKeys lists the keys matching pattern that the repository's own
// configuration sets: .git/config and what it includes, and, when
// extensions.worktreeConfig is on, the per-worktree config.worktree, which
// --local does not read and a command in a container could write. ok is
// false when git could not answer, which callers treat as a match.
func repoConfigKeys(ctx context.Context, env environment.Environment, pattern string) (keys []string, ok bool) {
	scopes := []string{"--local"}
	if out, _ := git(ctx, env, "config", "--local", "--type=bool", "--get", "extensions.worktreeConfig"); strings.TrimSpace(out) == "true" {
		scopes = append(scopes, "--worktree")
	}
	for _, scope := range scopes {
		out, code, answered := gitCode(ctx, env, "config", scope, "--includes", "--name-only", "--get-regexp", pattern)
		switch {
		case answered && code == 0:
			keys = append(keys, strings.Fields(out)...)
		case answered && code == 1: // nothing matches
		default:
			if _, isRepo := git(ctx, env, "rev-parse", "--git-dir"); isRepo {
				return keys, false
			}
		}
	}
	return keys, true
}

// gitCode is git that also returns the exit code; answered is false when
// git did not run to an exit.
func gitCode(ctx context.Context, env environment.Environment, args ...string) (string, int, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := env.Exec(ctx, environment.ExecSpec{
		Argv: gitArgvFor(ctx, env, args...),
		Env:  gitEnvFor(env, nil),
	})
	if err != nil || res.TimedOut {
		return "", 0, false
	}
	return res.Output, res.ExitCode, true
}

func parseStatus(out string) event.GitStateData {
	st := event.GitStateData{Repo: true}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			if oid := strings.TrimPrefix(line, "# branch.oid "); oid != "(initial)" {
				st.Head = oid
			}
		case strings.HasPrefix(line, "# branch.head "):
			if h := strings.TrimPrefix(line, "# branch.head "); h != "(detached)" {
				st.Branch = h
			}
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			st.Dirty++
		}
	}
	return st
}

// fingerprint identifies a repository across moves (OQ-020): its oldest
// root commit and the origin URL.
func fingerprint(ctx context.Context, env environment.Environment) (firstCommit, remote string) {
	if out, ok := git(ctx, env, "rev-list", "--max-parents=0", "HEAD"); ok {
		roots := strings.Fields(out)
		slices.Sort(roots)
		if len(roots) > 0 {
			firstCommit = roots[0]
		}
	}
	if out, ok := git(ctx, env, "config", "--get", "remote.origin.url"); ok {
		remote = strings.TrimSpace(out)
	}
	return firstCommit, remote
}

// sameGit reports two git states the same apart from the commits listed with them.
func sameGit(a, b event.GitStateData) bool {
	return a.Repo == b.Repo && a.Branch == b.Branch && a.Head == b.Head && a.Dirty == b.Dirty && a.BaseCommit == b.BaseCommit
}

// maxNewCommits caps the commits one HEAD move lists.
const maxNewCommits = 100

// newCommits lists the commits a checkout's HEAD moved forward over, from old to
// now, merges left out (GUI-051). A move that is not a fast-forward (an amend, a
// rebase, a reset, a switch of branch) lists none. Their lines are counted only in
// a trusted project whose config runs nothing; elsewhere only rev-list runs, which
// is allowed plumbing.
func newCommits(ctx context.Context, env environment.Environment, old, now string, trusted bool) []event.CommitInfo {
	if old == "" || now == "" || old == now {
		return nil
	}
	if _, ok := git(ctx, env, "merge-base", "--is-ancestor", old, now); !ok {
		return nil
	}
	out, ok := git(ctx, env, "rev-list", "--no-merges", "--timestamp", "--max-count="+strconv.Itoa(maxNewCommits), now, "^"+old)
	if !ok {
		return nil
	}
	var commits []event.CommitInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		at, sha, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		when, _ := strconv.ParseInt(at, 10, 64)
		commits = append(commits, event.CommitInfo{SHA: sha, At: when, Added: -1, Deleted: -1, Files: -1})
	}
	if len(commits) == 0 || !trusted || blocked(ctx, env) != "" {
		return commits
	}
	for i := range commits {
		out, ok := git(ctx, env, "diff-tree", "-r", "--root", "--no-commit-id", "--numstat", "--no-ext-diff", "--no-textconv", "--no-renames", commits[i].SHA)
		if !ok {
			continue
		}
		commits[i].Added, commits[i].Deleted, commits[i].Files = numstat(out)
	}
	return commits
}

// numstat adds up git's --numstat lines; a binary file counts as a file only.
func numstat(out string) (added, deleted, files int) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		files++
		a, errA := strconv.Atoi(f[0])
		d, errD := strconv.Atoi(f[1])
		if errA == nil && errD == nil {
			added += a
			deleted += d
		}
	}
	return added, deleted, files
}

// committed records a commit the app made for a session, the Diff tab's or an AFK
// run's, as that session's (GUI-051): before is HEAD before it.
func (e *Engine) committed(ctx context.Context, sid string, env environment.Environment, before string, trusted bool) {
	st := gitState(ctx, env, trusted)
	if !st.Repo || st.Head == "" || st.Head == before {
		return
	}
	st.Cause = event.CauseCommit
	st.Commits = newCommits(ctx, env, before, st.Head, trusted)
	e.mu.Lock()
	r, ok := e.runtimes[sid]
	e.mu.Unlock()
	if ok {
		r.gitMu.Lock()
		defer r.gitMu.Unlock()
		st.BaseCommit = r.base
		r.lastGit = st
		r.lastGit.Commits, r.lastGit.Cause = nil, ""
	} else if ses, err := e.store.GetSession(ctx, sid); err == nil {
		st.BaseCommit = ses.BaseCommit
	}
	e.emit(sid, event.GitState, st)
}
