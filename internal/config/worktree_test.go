package config

import (
	"slices"
	"testing"
)

func TestWorktreeScripts(t *testing.T) {
	w, err := ParseWorktreeScripts([]byte("copy: [.env, config/local.json]\nsetup: npm install\nrun: npm run dev -- --port $FREELOOP_PORT\n"))
	if err != nil || w.Setup != "npm install" || !slices.Equal(w.Copy, []string{".env", "config/local.json"}) {
		t.Fatalf("parsed %+v, %v", w, err)
	}
	app := WorktreeScripts{Setup: "pnpm install", Archive: "docker compose down"}
	got := w.Over(app)
	if got.Setup != "pnpm install" || got.Run != w.Run || got.Archive != "docker compose down" || len(got.Copy) != 2 {
		t.Errorf("merged %+v", got)
	}
	for _, bad := range []string{"copy: [/etc/passwd]\n", "copy: [../secret]\n", "copy: [.]\n", "setpu: x\n"} {
		if _, err := ParseWorktreeScripts([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if !(WorktreeScripts{}).Empty() || w.Empty() {
		t.Error("Empty is wrong")
	}
}

// Issue 064: what trusting a project approves of its worktree.yaml covers every field,
// the copy list included, for that project in that folder.
func TestProjectWorktreeFingerprint(t *testing.T) {
	w := WorktreeScripts{Copy: []string{".env"}, Setup: "npm install"}
	fp := ProjectWorktreeFingerprint("p", "/r", w)
	if fp == "" || ProjectWorktreeFingerprint("p", "/r", WorktreeScripts{Copy: []string{}}) != "" {
		t.Fatal("a file that sets something needs a mark, and one that sets nothing none")
	}
	changed := []WorktreeScripts{
		{Copy: []string{".env", "secrets.json"}, Setup: "npm install"},
		{Copy: []string{".env"}, Setup: "npm install", Run: "npm run dev"},
		{Copy: []string{".env"}, Setup: "npm install", Archive: "rm -rf ~"},
		{Copy: []string{".env"}, Setup: "npm ci"},
	}
	for _, c := range changed {
		if ProjectWorktreeFingerprint("p", "/r", c) == fp {
			t.Errorf("%+v has the same mark", c)
		}
	}
	if ProjectWorktreeFingerprint("q", "/r", w) == fp || ProjectWorktreeFingerprint("p", "/s", w) == fp {
		t.Error("another project or folder has the same mark")
	}
	if ProjectWorktreeFingerprint("p", "/r", WorktreeScripts{Setup: "x"}) != ProjectWorktreeFingerprint("p", "/r", WorktreeScripts{Copy: []string{}, Setup: "x"}) {
		t.Error("an empty copy list reads as none")
	}
}
