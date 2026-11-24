package engine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// SET-001: hand edits to the config files reach the engine within 2 s, and
// a broken file is reported, not fatal; fixing it clears the report.
func TestHotReload(t *testing.T) {
	dir := t.TempDir()
	changes := make(chan ConfigChange, 16)
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: 20 * time.Millisecond,
		OnConfigChange: func(c ConfigChange) { changes <- c }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: not reloaded within 2 s", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	issueIn := func(file string) bool { return hasIssue(e, file) }

	write("config.yaml", "version: 1\nui:\n  theme: light\n  fontSize: 16\n")
	wait("config.yaml", func() bool { return e.Config().UI.Theme == "light" })
	if c := <-changes; !slices.Contains(c.Files, "config.yaml") {
		t.Errorf("change = %+v", c)
	}

	write("config.yaml", "version: 1\nui: [not, a, map\n")
	wait("broken config.yaml", func() bool { return issueIn("config.yaml") })
	write("config.yaml", "version: 1\n")
	wait("fixed config.yaml", func() bool { return !issueIn("config.yaml") })

	write("providers.yaml", "version: 1\nproviders:\n  - id: local\n    name: Local\n    type: ollama\n    baseUrl: http://127.0.0.1:1\n")
	wait("providers.yaml", func() bool {
		return slices.ContainsFunc(e.ListProviders(), func(p ProviderInfo) bool { return p.ID == "local" })
	})

	write("rules.yaml", "version: 1\nrules:\n  - effect: maybe\n    tool: shell\n")
	wait("broken rules.yaml", func() bool { return issueIn("rules.yaml") })
	write("rules.yaml", "version: 1\nrules:\n  - effect: deny\n    tool: shell\n    command: git push *\n")
	wait("fixed rules.yaml", func() bool { return !issueIn("rules.yaml") && len(e.ListRules("")) == 1 })

	write("environments/ci.yaml", "name: CI\nkind: local\nenv:\n  CI: \"1\"\n")
	wait("environments", func() bool {
		return slices.ContainsFunc(e.ListEnvironments(), func(x EnvironmentInfo) bool { return x.ID == "ci" })
	})
}

func hasIssue(e *Engine, file string) bool {
	for _, i := range e.ConfigIssues() {
		if strings.HasSuffix(i.File, file) {
			return true
		}
	}
	return false
}

// Review finding: a config.yaml that could not be read was replaced by
// defaults on reload and overwritten by the next save.
func TestUnreadableConfigIsKept(t *testing.T) {
	dir := t.TempDir()
	e := openTest(t, dir)
	defer e.Close()
	cfg := config.Default()
	cfg.Network.LocalOnly = false
	cfg.UI.Theme = "dark"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	broken := []byte("ui: [unclosed\n")
	if err := os.WriteFile(filepath.Join(dir, config.FileName), broken, 0o600); err != nil {
		t.Fatal(err)
	}
	e.reload([]string{config.FileName})
	if e.Config().UI.Theme != "dark" {
		t.Errorf("theme after a broken edit = %q", e.Config().UI.Theme)
	}
	if _, err := e.SaveSettings(config.Default()); err == nil {
		t.Error("saved over a file that cannot be read")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, config.FileName)); string(b) != string(broken) {
		t.Error("the broken file was rewritten")
	}
}

// SET-001, issue 081: a hand-edited file applies only once it reads the same on two
// polls in a row, so a save caught half done (an editor that truncates the file, then
// writes it) never applies, not even to a permission check between polls; and a
// rules.yaml with errors never shrinks the deny rules in force.
func TestAHalfSavedFileIsNotApplied(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(Options{DataDir: dir, Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	w := e.newConfigWatch()
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	denies := func() []string {
		rules, _ := e.rules.Rules("")
		out := []string{}
		for _, r := range rules {
			if r.Effect == permission.Deny {
				out = append(out, r.Command)
			}
		}
		slices.Sort(out)
		return out
	}

	full := "version: 1\nrules:\n  - {effect: deny, tool: shell, command: \"git push *\"}\n  - {effect: deny, tool: shell, command: \"rm *\"}\n"
	write(config.RulesFileName, full)
	e.reloadChanged(w)
	if got := denies(); len(got) != 0 {
		t.Fatalf("applied on the first poll that saw it: %v", got)
	}
	e.reloadChanged(w)
	if got := denies(); len(got) != 2 {
		t.Fatalf("not applied on the second: %v", got)
	}

	// The editor truncates the file, a poll comes, and then it writes.
	write(config.RulesFileName, "")
	if got := denies(); len(got) != 2 {
		t.Errorf("a check between polls read the empty file: %v", got)
	}
	e.reloadChanged(w)
	if got := denies(); len(got) != 2 {
		t.Errorf("the empty file applied: %v", got)
	}
	write(config.RulesFileName, full+"  - {effect: deny, tool: shell, command: \"curl *\"}\n")
	e.reloadChanged(w)
	if got := denies(); len(got) != 2 {
		t.Errorf("applied while still changing: %v", got)
	}
	e.reloadChanged(w)
	if got := denies(); len(got) != 3 {
		t.Errorf("the finished file did not apply: %v", got)
	}

	// A file with errors keeps every deny rule in force, and the broker fails closed.
	write(config.RulesFileName, "version: 1\nrules:\n  - {effect: deny, tool: shell, command: \"rm *\"}\n  - {effect: deny, tool: shell, comand: typo}\n")
	e.reloadChanged(w)
	e.reloadChanged(w)
	if got := denies(); !slices.Equal(got, []string{"curl *", "git push *", "rm *"}) {
		t.Errorf("deny rules after a file with errors: %v", got)
	}
	if _, err := e.rules.Rules(""); err == nil {
		t.Error("the broker is not told that rules.yaml has errors")
	}

	// A config.yaml cut short is valid, and would bring the defaults back for a poll.
	write(config.FileName, "version: 1\nui:\n  theme: light\n  fontSize: 16\n")
	e.reloadChanged(w)
	e.reloadChanged(w)
	if e.Config().UI.Theme != "light" {
		t.Fatalf("theme = %q", e.Config().UI.Theme)
	}
	write(config.FileName, "version: 1\n")
	e.reloadChanged(w)
	write(config.FileName, "version: 1\nui:\n  theme: dark\n  fontSize: 16\n")
	e.reloadChanged(w)
	if e.Config().UI.Theme != "light" {
		t.Errorf("a half-saved config.yaml applied: theme %q", e.Config().UI.Theme)
	}
	e.reloadChanged(w)
	if e.Config().UI.Theme != "dark" {
		t.Errorf("the saved config.yaml did not apply: theme %q", e.Config().UI.Theme)
	}

	// A file deleted applies once it stays deleted.
	os.Remove(filepath.Join(dir, config.RulesFileName))
	e.reloadChanged(w)
	if got := denies(); len(got) != 3 {
		t.Errorf("a deletion applied on the first poll: %v", got)
	}
	e.reloadChanged(w)
	if got := denies(); len(got) != 0 {
		t.Errorf("the deletion did not apply: %v", got)
	}
}
