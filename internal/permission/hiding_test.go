package permission

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// modesToTry are where a shell deny rule is all that stands between the model and a
// command: an allow rule in ask and accept-edits mode, plan mode, bypass mode, and the
// sandbox, which needs no approval, in each mode it runs in.
var modesToTry = []struct {
	mode      Mode
	contained Containment
}{
	{ModeAsk, ""}, {ModeAcceptEdits, ""}, {ModePlan, ""}, {ModeBypass, ""},
	{ModeAsk, ContainedProject}, {ModeAcceptEdits, ContainedProject}, {ModeBypass, ContainedProject},
	{ModeAsk, ContainedReadOnly}, {ModePlan, ContainedReadOnly},
}

// SEC-003: a line that hides its command from a shell deny rule is never allowed, in any
// mode and in the sandbox: the rule denies it, bypass mode refuses it, or the user is
// asked. Found in the review of 2026-11-23 by probes against Broker.evaluate (issue 068),
// with more of their kinds found while fixing: before the fix, every line but env -iS was
// allowed in at least one of these modes, by the sandbox, by plan mode in a read-only
// sandbox, by an allow rule or by bypass mode.
func TestAShellDenyRuleHoldsWhereNothingElseAsks(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	root := filepath.Join(home, "repo")
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	for _, c := range []struct {
		deny, program string
		lines         []string
	}{
		{"curl *", "curl", []string{
			// A glob in the program, a shell reading its input, env's split string,
			// zsh's repeat, busybox, find's -exec, time with options.
			"cur? x", "/usr/bin/cur* x", "echo curl x | sh", "env -S 'curl x'", "repeat 1 curl x",
			"busybox sh -c 'curl x'", "find . -name x -exec curl y +", "time -p curl x",
			// A variable as the program, in plan mode's read-only sandbox.
			"c=curl; $c http://localhost:1",
		}},
		{"git push *", "git", []string{
			// A glob in the subcommand, and an alias set on the line.
			"git pu*h origin", "git -C . pu?h", "git {push,} origin", "git -c alias.p=push p origin", "git -c Alias.p=push p",
			"git --config-env=alias.p=P p origin", "git --config-env alias.p=P p", "git -c include.path=x.cfg p",
			`git -c "$cfg" p`, "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=alias.p GIT_CONFIG_VALUE_0=push git p",
			"git -C . $sub origin",
			// Redirections before the command, or anywhere in it.
			"</dev/null git push", "2>&1 git push", "2> /dev/null git push", "git 2>/dev/null push", "&>/dev/null git push",
			"true &>/dev/null git push",
			// Shells: options with values before -c, input, a here-string, - and -s.
			"bash -O extglob -c 'git push'", "bash +O extglob -o pipefail -c 'git push'", "zsh -o extendedglob -c 'git push'",
			"echo 'git push' | sh", "sh <<< 'git push'", "bash <<<'git push origin'", "echo git push | bash -", "echo git push | bash -s",
			"echo git push | zsh -x", "fish --command='git push'", "fish -C 'git push'", "fish --init-command='git push'",
			// env -S, coproc, zsh's repeat and -, time with options.
			"env -S 'git push'", "env -S'git push'", "env -iS 'git push'", "env --split-string='git push'", `env -S 'git\_push'`,
			"coproc git push", "coproc p { git push; }", "repeat 2 git push", "repeat 2 do git push; done", "repeat -1 git push", "- git push",
			"time -p git push", "busybox git push", "find . -execdir git push ';'", "diff <(git push) x",
			// A shell alias or function defined on the line.
			"alias p='git push'\np origin", "alias p=git; p push", "f() { git push; }; f", "f(){ git push; }; f", "function f { git push; }; f",
		}},
	} {
		rules := fixedRules{
			{Effect: Deny, Tool: "shell", Command: c.deny, Scope: ScopeGlobal},
			{Effect: Allow, Tool: "shell", Command: c.program + " *", Scope: ScopeGlobal},
			{Effect: Allow, Tool: "shell", Scope: ScopeGlobal},
		}
		for _, line := range c.lines {
			for _, m := range modesToTry {
				b.SetRules(rules, nil)
				req := shellReq(line)
				req.Contained = m.contained
				d, decided := b.evaluate(Context{Mode: m.mode, Root: root}, req)
				switch {
				case decided && d.Outcome == Allowed:
					t.Errorf("deny %s: %q in %s mode (%s) was allowed: %s, %s", c.deny, line, m.mode, m.contained, d.Actor, d.Reason)
				case m.mode == ModeBypass && !decided:
					t.Errorf("deny %s: %q in bypass mode (%s) asked", c.deny, line, m.contained)
				}
				// With no deny rule, nothing is hidden from one: the line runs as before.
				b.SetRules(fixedRules{}, nil)
				d, decided = b.evaluate(Context{Mode: m.mode, Root: root}, req)
				switch {
				case m.mode == ModeBypass, m.contained == ContainedProject, m.contained == ContainedReadOnly:
					if !decided || d.Outcome != Allowed {
						t.Errorf("no deny rule: %q in %s mode (%s): %+v, %v", line, m.mode, m.contained, d, decided)
					}
				}
			}
		}
	}
}

// What hides nothing from the rules in play runs as before: in the sandbox and in bypass
// mode a plain line runs, and a glob only hides a word it could spell.
func TestLinesThatHideNothingStillRun(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	root := filepath.Join(home, "repo")
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	b.SetRules(fixedRules{
		{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal},
		{Effect: Deny, Tool: "shell", Command: "rm -rf *", Scope: ScopeGlobal},
		{Effect: Deny, Tool: "shell", Command: "curl *", Scope: ScopeGlobal},
	}, nil)
	for _, line := range []string{
		"git status", "go test ./...", "ls *.go", "git log -- '*.go'", "git add *.go", "git -c color.ui=false log", "git -C sub status",
		"git -c user.name=x commit -m msg", "rm *.o", "rm -f *.tmp", "time go test ./...", "time -p go test", "sh -c 'git status'",
		"bash script.sh", "bash -O extglob -c 'git log'", "sh --version", "env -S 'git status'", "coproc git status", "repeat 3 git fetch",
		"find . -name x -exec gofmt -l x ';'", "busybox ls", "echo curl x", "git pu", "bash -c 'git pull'",
	} {
		for _, m := range modesToTry {
			if m.mode == ModeBypass || m.contained != "" {
				req := shellReq(line)
				req.Contained = m.contained
				if d, decided := b.evaluate(Context{Mode: m.mode, Root: root}, req); !decided || d.Outcome != Allowed {
					t.Errorf("%q in %s mode (%s): %+v, %v", line, m.mode, m.contained, d, decided)
				}
			}
		}
	}
	// `*` may spell -rf, which the rule on rm names, while *.o cannot.
	if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq("rm *")); !decided || d.Outcome != Denied {
		t.Errorf("rm * in bypass mode: %+v, %v", d, decided)
	}
}

// A deny rule can name a wrapper itself, as it can name rtk: `deny shell(sudo *)` stops
// sudo whatever it runs (found while fixing issue 068: the wrapper was taken off the line
// before any rule saw it). Through busybox, which now unwraps, a rule on it still holds.
func TestADenyRuleOnAWrapperHolds(t *testing.T) {
	for _, c := range []struct{ rule, line string }{
		{"sudo *", "sudo rm x"}, {"sudo rm *", `sudo "rm" x`}, {"env *", "env A=1 make"}, {"busybox *", "busybox sh -c 'ls'"},
		{"nohup *", "nohup ./server"}, {"rtk *", "rtk git status"},
	} {
		if !(Rule{Effect: Deny, Tool: "shell", Command: c.rule}).Matches(shellReq(c.line), "/p") {
			t.Errorf("deny %s does not stop %q", c.rule, c.line)
		}
	}
}

// What a line costs to read grows with its length, not its square: a wrapper's own line
// is kept for the first maxWrappers of them, and past that the line is unseen, and find
// follows each -exec once (a long chain of either would otherwise hold the broker up).
func TestLongChainsStayCheap(t *testing.T) {
	if out, unseen := commandsOf(strings.Repeat("nohup ", 5000)+"ls", 0); len(out) > maxWrappers+1 || len(unseen) != 1 {
		t.Errorf("5000 wrappers: %d commands, unseen %q", len(out), unseen)
	}
	if out, _ := commandsOf(strings.Repeat("find . -exec ", 2000)+"ls", 0); len(out) > 2*maxDepth {
		t.Errorf("2000 nested -exec: %d commands", len(out))
	}
	// A rule on a wrapper past the first maxWrappers still holds: the line asks.
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	b.SetRules(fixedRules{{Effect: Deny, Tool: "shell", Command: "env *", Scope: ScopeGlobal}}, nil)
	line := strings.Repeat("nohup ", maxWrappers) + "env make"
	if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: filepath.Join(home, "repo")}, shellReq(line)); !decided || d.Outcome != Denied {
		t.Errorf("env after %d wrappers in bypass mode: %+v, %v", maxWrappers, d, decided)
	}
}

// An execute request whose command the client could not read, an external agent's given
// in a shape it does not know, showed a shell deny rule nothing to compare, so an allow
// rule for every command and bypass mode let it past `deny shell(git push *)` (issue
// 079). While such a rule exists it is unseen: nothing that only a rule or the mode
// allows lets it run. With none, it is decided as before.
func TestAnExecuteWithNoCommandIsUnseen(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	root := filepath.Join(home, "repo")
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	allowAll := Rule{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}
	for _, command := range []string{"", " \t"} {
		for _, m := range modesToTry {
			req := Request{Tool: "shell", Kind: KindExecute, Command: command, Contained: m.contained}
			b.SetRules(fixedRules{{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}, allowAll}, nil)
			d, decided := b.evaluate(Context{Mode: m.mode, Root: root}, req)
			switch {
			case decided && d.Outcome == Allowed:
				t.Errorf("%q in %s mode (%s) was allowed: %s, %s", command, m.mode, m.contained, d.Actor, d.Reason)
			case m.mode == ModeBypass && (!decided || d.Outcome != Denied):
				t.Errorf("%q in bypass mode (%s): %+v, %v", command, m.contained, d, decided)
			}
			b.SetRules(fixedRules{allowAll}, nil)
			d, decided = b.evaluate(Context{Mode: m.mode, Root: root}, req)
			if m.mode != ModePlan || m.contained != "" {
				if !decided || d.Outcome != Allowed {
					t.Errorf("no deny rule: %q in %s mode (%s): %+v, %v", command, m.mode, m.contained, d, decided)
				}
			}
		}
	}
}
