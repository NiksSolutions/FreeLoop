package permission

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

func shellReq(cmd string) Request { return Request{Tool: "shell", Kind: KindExecute, Command: cmd} }

// A deny rule compares the words the shell will run, not the way they are spelled: every
// word is read with its quotes and escapes removed, and zsh's own prefixes are wrappers.
// Reproduced on 2026-11-11: with `deny git push *` and `allow git *`, `git "push" origin
// main` was allowed by the rule in ask mode (SEC-003).
func TestShellDenyRulesSeeThroughQuotedWordsAndZshPrefixes(t *testing.T) {
	r := Rule{Effect: Deny, Tool: "shell", Command: "git push *"}
	for _, line := range []string{
		`git push origin main`, `git "push" origin main`, `git 'push' origin`, `git pu""sh origin`, `git p\ush origin`,
		`git "p"'u'sh origin`, `"git" "push" origin`, `git -C . "push"`, `git -c a=b --no-pager 'push' -f`,
		`noglob git push origin`, `nocorrect git push origin`, `=git push origin`, `command git push origin`,
		`builtin command git push`, `env A=1 git "push"`, `sudo -u root git "push"`,
		`eval "git push origin"`, `eval git push origin`, `eval 'git "push" origin'`,
		`sh -c 'git "push" origin'`, `echo hi && git "push"`, `(git "push")`, `{ git "push"; }`,
	} {
		if !r.Matches(shellReq(line), "/p") {
			t.Errorf("%q got past the rule", line)
		}
	}
	for _, line := range []string{`git pull`, `git "pull" origin`, `echo git push`, `git status`, `echo "git push"`, `git log --grep push`} {
		if r.Matches(shellReq(line), "/p") {
			t.Errorf("%q matched the rule", line)
		}
	}
}

// Flag letters are read as a set, in any order and split or joined, and rm's long names
// are its short ones; a command with more flags than the rule names is still the command.
func TestShellDenyRulesReadFlagsAsASet(t *testing.T) {
	r := Rule{Effect: Deny, Tool: "shell", Command: "rm -rf *"}
	for _, line := range []string{
		`rm -rf x`, `rm -fr x`, `rm -r -f x`, `rm -f -r x`, `rm -Rf x`, `rm --recursive --force x`, `rm --force -r x`,
		`rm -rfv x`, `rm -v -rf x`, `rm -r -v -f x`, `sudo rm -fr /`, `rm "-fr" x`, `rm -rf`,
	} {
		if !r.Matches(shellReq(line), "/p") {
			t.Errorf("%q got past the rule", line)
		}
	}
	for _, line := range []string{`rm -r x`, `rm -f x`, `rm x`, `rm --recursive x`, `git rm -rf x`, `echo rm -rf x`} {
		if r.Matches(shellReq(line), "/p") {
			t.Errorf("%q matched the rule", line)
		}
	}
	// A rule that names no flags is unchanged, and so is an exact rule.
	if !(Rule{Effect: Deny, Tool: "shell", Command: "git push -f *"}).Matches(shellReq(`git push -f origin`), "/p") {
		t.Error("git push -f")
	}
	if (Rule{Effect: Deny, Tool: "shell", Command: "git push --force-with-lease"}).Matches(shellReq(`git push --force`), "/p") {
		t.Error("--force-with-lease is not --force")
	}
}

// A command whose program, or the subcommand after it, is built when it runs cannot be
// compared with a deny rule. It is never allowed by a rule that allows commands, and
// bypass mode refuses it while a shell deny rule exists; with none it runs as before.
func TestACommandBuiltAtRunTimeIsNotAllowedUnseenWhileADenyRuleExists(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	root := filepath.Join(home, "repo")
	deny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}
	allowShell := Rule{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}
	for _, line := range []string{`a=git; $a push origin`, `git "$sub" origin`, "$(echo git) push", "`echo git` push", `g=git; "$g" push`, `git $'push' origin`, `${cmd} push`, `g*t push origin`, `/usr/bin/gi[t] push`, `/usr/bin/g?t push`} {
		b.SetRules(fixedRules{deny, allowShell}, nil)
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); decided {
			t.Errorf("%q in ask mode with allow shell was decided: %+v", line, d)
		}
		if d, decided := b.evaluate(Context{Mode: ModeAcceptEdits, Root: root}, shellReq(line)); decided {
			t.Errorf("%q in accept-edits mode with allow shell was decided: %+v", line, d)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Denied {
			t.Errorf("%q in bypass mode: %+v, %v", line, d, decided)
		}
		// With no shell deny rule nothing needs to be compared.
		b.SetRules(fixedRules{allowShell}, nil)
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q with no deny rule: %+v, %v", line, d, decided)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q in bypass with no deny rule: %+v, %v", line, d, decided)
		}
	}
	// Variables and expansions in the arguments are everyday and still run.
	b.SetRules(fixedRules{deny, allowShell}, nil)
	for _, line := range []string{`echo $HOME`, `git commit -m "$msg"`, `ls $(pwd)`, `FOO=$(date) make`, `git status > out.txt`, `cat < in.txt | wc -l`, `[ -f go.mod ] && echo ok`, `[[ -n $x ]] && echo ok`, `ls *.go`, `echo ?`} {
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q: %+v, %v", line, d, decided)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q in bypass: %+v, %v", line, d, decided)
		}
	}
}

// A deny rule on one of the two ways to write a file holds for the other, and for an
// external agent's write, which is judged as an edit: the model chooses the tool, the
// user's rule is about the path. Reproduced on 2026-11-11: `deny write_file(secrets/**)`
// did not stop edit_file (SEC-003). An allow rule stays as narrow as it was written.
func TestAPathDenyRuleOnAWriteToolCoversTheOther(t *testing.T) {
	root := "/p"
	req := func(tool string) Request {
		return Request{Tool: tool, Kind: KindEdit, Paths: []string{"/p/secrets/key.txt"}}
	}
	for _, c := range []struct{ rule, tool string }{
		{"write_file", "edit_file"}, {"edit_file", "write_file"}, {"write_file", "write_file"}, {"edit_file", "edit_file"},
		{"write_*", "edit_file"}, {"*", "edit_file"},
	} {
		r := Rule{Effect: Deny, Tool: c.rule, Path: "secrets/**"}
		if !r.Matches(req(c.tool), root) {
			t.Errorf("deny %s(secrets/**) does not hold for %s", c.rule, c.tool)
		}
	}
	for _, c := range []struct{ rule, tool string }{
		{"read_file", "edit_file"}, {"grep", "write_file"}, {"shell", "edit_file"}, {"write_file", "read_file"}, {"write_file", "grep"},
	} {
		r := Rule{Effect: Deny, Tool: c.rule, Path: "secrets/**"}
		if r.Matches(Request{Tool: c.tool, Kind: KindEdit, Paths: []string{"/p/secrets/key.txt"}}, root) {
			t.Errorf("deny %s(secrets/**) holds for %s", c.rule, c.tool)
		}
	}
	// An allow rule is only for the tool it names.
	allow := Rule{Effect: Allow, Tool: "write_file", Path: "src/**"}
	if allow.Matches(Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{"/p/src/a.go"}}, root) {
		t.Error("allow write_file(src/**) allows edit_file")
	}
	// And a deny rule with a command is unchanged.
	if (Rule{Effect: Deny, Tool: "write_file", Command: "x"}).Matches(Request{Tool: "edit_file", Command: "x"}, root) {
		t.Error("a command rule on write_file matched edit_file")
	}
}

// Through the broker: a deny rule on write_file stops an external agent's edit, even in
// bypass mode, and `freeloop check` says so.
func TestAWriteDenyRuleStopsAnExternalAgentsEdit(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	root := filepath.Join(home, "repo")
	b.SetRules(fixedRules{{Effect: Deny, Tool: "write_file", Path: "secrets/**", Scope: ScopeGlobal}}, nil)
	for _, mode := range []Mode{ModeAsk, ModeAcceptEdits, ModeBypass} {
		d, decided := b.evaluate(Context{Mode: mode, Root: root}, Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "secrets", "k.txt")}})
		if !decided || d.Outcome != Denied || d.Actor != ActorRule {
			t.Errorf("%s: %+v, %v", mode, d, decided)
		}
	}
	if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{filepath.Join(root, "src", "a.go")}}); !decided || d.Outcome != Allowed {
		t.Errorf("an edit elsewhere: %+v, %v", d, decided)
	}
}

// A deny rule names a program, not a spelling of it: git.exe, a path with either slash,
// GIT on a volume that ignores case (APFS, NTFS), a shell, cmd or PowerShell that runs
// the line. Reproduced on 2026-11-13 with `deny git push *` in bypass mode: the first
// lines below were allowed by the mode (SEC-003).
func TestShellDenyRulesSeeOtherSpellingsOfTheProgram(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	root := filepath.Join(home, "repo")
	deny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}
	allowShell := Rule{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}
	b.SetRules(fixedRules{deny}, nil)
	for _, line := range []string{
		`git push origin main`, `git.exe push origin main`, `/mingw64/bin/git.exe push origin main`, `GIT push origin main`,
		`Git.EXE push origin main`, `bash.exe -c "git push origin main"`, `cmd //c git push origin main`,
		`powershell -Command "git push origin main"`,
		`'C:\Git\cmd\git.exe' push origin`, `C:/Git/cmd/GIT.CMD push`, `git.bat push`, `git.com push`, `GIT -C . push`,
		`/usr/bin/BASH -lc 'git push'`, `C:/Git/bin/sh.exe -c "git push"`, `cmd /c git push origin main`,
		`cmd.exe /C "git push origin main"`, `CMD /d /s /c "git push"`, `cmd /k git push`, `pwsh -c 'git push'`,
		`powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "git push"`, `pwsh -NoLogo -Com "git.exe push"`,
		`powershell git push origin`, `ENV.EXE git push`, `/usr/bin/Sudo git push`,
	} {
		if !deny.Matches(shellReq(line), root) {
			t.Errorf("%q got past the rule", line)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Denied {
			t.Errorf("%q in bypass mode: %+v, %v", line, d, decided)
		}
	}
	// A rule written with another spelling is the same rule.
	if !(Rule{Effect: Deny, Tool: "shell", Command: "Git.exe push *"}).Matches(shellReq("git push origin"), root) {
		t.Error("deny Git.exe push * does not stop git push")
	}
	// What a line hides from the rules asks, and bypass refuses it: an encoded
	// PowerShell command, a cmd line with its escapes, variables and own launchers, a
	// PowerShell line that builds, calls or renames what it runs, an option not known.
	for _, line := range []string{
		`powershell -EncodedCommand ZwBpAHQAIABwAHUAcwBoAA==`, `pwsh -enc ZwBpAHQA`, `powershell -e ZwBpAHQA`, `pwsh -ec ZwBpAHQA`,
		`cmd /c "g^it push"`, `cmd /c %G% push`, `cmd /c call git push`, `cmd /c start git push`, `cmd /v:on /c "!g! push"`,
		`powershell -Command "& ('gi'+'t') push"`, `pwsh -c "iex 'git push'"`, `pwsh -c "Invoke-Expression 'git push'"`,
		"pwsh -c \"g`it push\"", `pwsh -c "Set-Alias g git; g push"`, `powershell -NoProfile -Unknown x`,
		`pwsh -c "Start-Process git -ArgumentList push"`, `pwsh -c '. git push'`,
	} {
		b.SetRules(fixedRules{deny, allowShell}, nil)
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); decided {
			t.Errorf("%q in ask mode with allow shell was decided: %+v", line, d)
		}
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Denied {
			t.Errorf("%q in bypass mode: %+v, %v", line, d, decided)
		}
	}
	// Other commands still run.
	b.SetRules(fixedRules{deny}, nil)
	for _, line := range []string{
		`git.exe status`, `GIT pull`, `cmd /c git status`, `cmd //c dir`, `powershell -Command "git status"`, `pwsh -NoProfile -c Get-ChildItem`,
		`bash.exe -c "git log"`, `echo git push`, `gitk push`, `git-push origin`, `echo 100%`,
	} {
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); !decided || d.Outcome != Allowed {
			t.Errorf("%q in bypass mode: %+v, %v", line, d, decided)
		}
	}
	// An allow rule stays as written: `allow git *` approves git, not GIT or git.exe,
	// which on a volume that tells case apart may be another program.
	b.SetRules(fixedRules{{Effect: Allow, Tool: "shell", Command: "git *", Scope: ScopeGlobal}}, nil)
	if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq("git status")); !decided || d.Outcome != Allowed {
		t.Errorf("git status: %+v, %v", d, decided)
	}
	for _, line := range []string{`GIT status`, `git.exe status`, `/tmp/x/git status`} {
		if d, decided := b.evaluate(Context{Mode: ModeAsk, Root: root}, shellReq(line)); decided {
			t.Errorf("%q was decided by allow git *: %+v", line, d)
		}
	}
}

// A backslash before a line break joins the lines before the shell reads them, so a
// deny rule sees the command they make together. Found by FuzzDenyRuleHoldsThroughWrappers
// on 2026-11-20: with deny git push * and allow shell, `git \⏎ push origin` was allowed
// in bypass mode, at the top of a line and inside zsh -c (SEC-003).
func TestDenyRulesSeeThroughLineContinuations(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	root := filepath.Join(home, "repo")
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	b.SetRules(fixedRules{{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}, {Effect: Allow, Tool: "shell", Scope: ScopeGlobal}}, nil)
	for _, line := range []string{
		"git \\\n push origin", "git \\\r\n push origin", "/usr/bin/git \\\n push origin", "gi\\\nt push origin",
		"zsh -c 'git \\\n push origin'", "bash -c \"git \\\n push\"", "env \\\n git push",
		// An escaped backslash ends with it: the line break is a real one, and git push
		// a command of its own.
		"echo \\\\\ngit push origin",
	} {
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); decided && d.Outcome == Allowed {
			t.Errorf("%q was allowed in bypass mode", line)
		}
	}
	// A subshell or a group after a keyword or a launcher, found by the same fuzzing.
	for _, line := range []string{
		"! (git push origin main)", "time (git push)", "! { git push; }", "if (git push); then :; fi",
		"while (git push); do :; done", "env (git push)", "nice ( git push )",
	} {
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); decided && d.Outcome == Allowed {
			t.Errorf("%q was allowed in bypass mode", line)
		}
	}
	// Deeper than the rules follow, a line is one they cannot see, so bypass refuses it:
	// four shells or evals deep got past the rule (found by the same fuzzing).
	sq := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	for _, wrap := range []string{"bash -c ", "eval ", "sh -c "} {
		line := "git push origin main"
		for range 6 {
			line = wrap + sq(line)
			if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); decided && d.Outcome == Allowed {
				t.Errorf("%q was allowed in bypass mode", line)
			}
		}
	}
	// A shell's line quoted inside another's, with an operator in it.
	for _, line := range []string{
		`bash -c 'bash -c '\''(true && git -C . push)'\'''`,
		`sh -c "sh -c 'true; git push origin'"`,
		`bash -c 'eval '\''true || git push'\'''`,
	} {
		if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq(line)); decided && d.Outcome == Allowed {
			t.Errorf("%q was allowed in bypass mode", line)
		}
	}
	// A continuation in a command a rule does not cover changes nothing.
	if d, decided := b.evaluate(Context{Mode: ModeBypass, Root: root}, shellReq("git log \\\n --oneline")); !decided || d.Outcome != Allowed {
		t.Errorf("git log over two lines: %+v, %v", d, decided)
	}
}
