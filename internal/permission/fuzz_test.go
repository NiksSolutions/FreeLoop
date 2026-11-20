package permission

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// SEC-002, SEC-003: a shell command a deny rule stops is still stopped, in bypass mode,
// however it is wrapped in what a shell sees through: a launcher (env, sudo, time,
// nice, nohup, command, exec), an assignment, a list, a group, a subshell, a command
// substitution, another shell's -c, blanks and line continuations. The fuzzer picks the
// command and the wrappings; asking the user is fine, allowing it is not.
func FuzzDenyRuleHoldsThroughWrappers(f *testing.F) {
	for _, seed := range [][]byte{{0}, {1, 0}, {2, 9, 5}, {0, 10, 11, 7}, {3, 8, 6, 12, 1}, {1, 13, 14, 15, 16, 17}, {2, 18, 19, 20, 21, 22}} {
		f.Add(seed)
	}
	home := environment.Canonical(f.TempDir())
	root := filepath.Join(home, "repo")
	deny := Rule{Effect: Deny, Tool: "shell", Command: "git push *", Scope: ScopeGlobal}
	allowShell := Rule{Effect: Allow, Tool: "shell", Scope: ScopeGlobal}
	b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
	b.SetRules(fixedRules{deny, allowShell}, nil)
	bases := []string{`git push origin main`, `git -C . push`, `/usr/bin/git push origin`, `GIT push --force`}
	sq := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	// Only lines a shell would run: a launcher takes a program, not a group, a
	// subshell, an if or a !, so one of those it would only fail to start goes into a
	// subshell's $() first; and ! cannot follow a pipe.
	compound := func(c string) bool {
		c = strings.TrimSpace(c)
		return strings.HasPrefix(c, "(") || strings.HasPrefix(c, "{") || strings.HasPrefix(c, "if ") || strings.HasPrefix(c, "!")
	}
	launch := func(word string) func(string) string {
		return func(c string) string {
			if compound(c) {
				c = "$(" + c + ")"
			}
			return word + c
		}
	}
	wraps := []func(string) string{
		launch("env "),
		launch("sudo "),
		launch("time "),
		launch("command "),
		launch("FOO=1 "),
		func(c string) string { return "true && " + c },
		func(c string) string { return c + "; true" },
		func(c string) string { return "(" + c + ")" },
		func(c string) string { return "$(" + c + ")" },
		func(c string) string {
			if strings.Contains(c, "`") {
				return "(" + c + ")"
			}
			return "`" + c + "`"
		},
		func(c string) string { return "bash -c " + sq(c) },
		func(c string) string { return "sh -c " + sq(c) },
		func(c string) string { return " \t" + c + "  " },
		launch("nice -n 5 "),
		launch("exec "),
		func(c string) string { return "{ " + c + "; }" },
		func(c string) string { return "if true; then " + c + "; fi" },
		launch("nohup "),
		func(c string) string { return strings.Replace(c, " ", " \\\n ", 1) },
		func(c string) string { return "zsh -c " + sq(c) },
		func(c string) string { return "! " + c },
		func(c string) string { return "eval " + sq(c) },
		func(c string) string {
			if strings.HasPrefix(strings.TrimSpace(c), "!") {
				c = "(" + c + ")"
			}
			return "echo x | " + c
		},
	}
	f.Fuzz(func(t *testing.T, picks []byte) {
		if len(picks) == 0 || len(picks) > 8 {
			return
		}
		cmd := bases[int(picks[0])%len(bases)]
		for _, p := range picks[1:] {
			cmd = wraps[int(p)%len(wraps)](cmd)
		}
		for _, mode := range []Mode{ModeBypass, ModeAsk} {
			if d, decided := b.evaluate(Context{Mode: mode, Root: root}, shellReq(cmd)); decided && d.Outcome == Allowed {
				t.Fatalf("%q was allowed in %s mode past deny git push *", cmd, mode)
			}
		}
	})
}

// SEC-003: no rule and no command makes the rules or the broker panic.
func FuzzRulesNeverPanic(f *testing.F) {
	for _, seed := range [][2]string{{"git push *", "git push origin"}, {"rm -rf *", `bash -c "rm -rf /"`}, {"*", "powershell -enc ZwBpAHQA"}, {"git *", "cmd /c \"g^it push\""}, {"[", "'unterminated"}} {
		f.Add(seed[0], seed[1])
	}
	home := environment.Canonical(f.TempDir())
	root := filepath.Join(home, "repo")
	f.Fuzz(func(t *testing.T, pattern, cmd string) {
		r := Rule{Effect: Deny, Tool: "shell", Command: pattern, Scope: ScopeGlobal}
		if r.Validate() != nil {
			return
		}
		r.Matches(shellReq(cmd), root)
		b := NewBroker(NewPolicy(home, filepath.Join(home, "data")), nil)
		b.SetRules(fixedRules{r, {Effect: Allow, Tool: "shell", Scope: ScopeGlobal}}, nil)
		for _, mode := range []Mode{ModeBypass, ModeAsk} {
			b.evaluate(Context{Mode: mode, Root: root}, shellReq(cmd))
		}
	})
}

// SEC-005: on a file system that ignores case, a protected location is protected in
// any case: ~/.SSH/ID_ED25519 is ~/.ssh/id_ed25519.
func FuzzProtectedPathsHoldInAnyCase(f *testing.F) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		f.Skip("this system's file systems tell cases apart")
	}
	for _, seed := range []uint64{0, 1, 0xff, 0xdeadbeef, ^uint64(0)} {
		f.Add(seed)
	}
	home := environment.Canonical(f.TempDir())
	data := filepath.Join(home, "data")
	p := NewPolicy(home, data)
	paths := []string{filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(home, ".aws", "credentials"), filepath.Join(data, "config.yaml")}
	f.Fuzz(func(t *testing.T, mask uint64) {
		for _, path := range paths {
			// Only the part under the home folder changes case; the home folder's own
			// spelling is what the system reports.
			rel := []rune(strings.TrimPrefix(path, home))
			for i, r := range rel {
				if mask>>(uint(i)%64)&1 == 1 {
					if unicode.IsLower(r) {
						rel[i] = unicode.ToUpper(r)
					} else {
						rel[i] = unicode.ToLower(r)
					}
				}
			}
			variant := home + string(rel)
			if p.readRefusal(variant) == "" {
				t.Fatalf("%s is readable", variant)
			}
			if p.writeRefusal(filepath.Join(home, "repo"), variant) == "" {
				t.Fatalf("%s is writable", variant)
			}
		}
	})
}
