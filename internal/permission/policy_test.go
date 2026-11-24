package permission

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// NTFS's other spellings of a folder are the folder: a stream (.git::$INDEX_ALLOCATION),
// trailing dots and spaces, which Windows drops, and an 8.3 short name (GIT~1). An
// external agent's edit is decided here and written by the agent, so the policy must
// read them as Windows does. Reproduced on 2026-11-13 in accept-edits mode: each was
// allowed (SEC-005). Refused on every system, where no one names a folder so.
func TestProtectedFoldersInEveryNTFSSpelling(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, "data"))
	b := NewBroker(p, nil)
	root := filepath.Join(home, "repo")
	for _, rel := range []string{
		".git/hooks/pre-commit", ".git::$INDEX_ALLOCATION/hooks/pre-commit", ".git./hooks/pre-commit", "GIT~1/hooks/pre-commit",
		".freeloop::$INDEX_ALLOCATION/hooks.yaml", ".git:x", ".git. . /config", "sub/.GIT ./config", ".freeloop./rules.yaml",
		"FREELO~1/hooks.yaml", "GI3F2A~1/config", "sub/git~2/config",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if p.writeRefusal(root, path) == "" {
			t.Errorf("%s is writable", rel)
		}
		d, decided := b.evaluate(Context{Mode: ModeAcceptEdits, Root: root}, Request{Tool: "edit_file", Kind: KindEdit, Paths: []string{path}})
		if !decided || d.Outcome != Denied || d.Actor != ActorProtectedPath {
			t.Errorf("%s in accept-edits mode: %+v, %v", rel, d, decided)
		}
	}
	// What a later tool acts on asks in any spelling too.
	for _, rel := range []string{".vscode::$INDEX_ALLOCATION/settings.json", ".vscode./tasks.json", "VSCODE~1/tasks.json", ".envrc.", "sub/.envrc::$DATA"} {
		if !p.actedOn(root, []string{filepath.Join(root, filepath.FromSlash(rel))}) {
			t.Errorf("%s is not acted on later", rel)
		}
	}
	for _, rel := range []string{"src/a.go", "PROGRA~1/x", "src/notes:1.txt", "gitignore.txt", "docs/v1./a.md", ".gitignore", "a~b/c"} {
		if why := p.writeRefusal(root, filepath.Join(root, filepath.FromSlash(rel))); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
}

// Linux's keyrings, browser profiles and password stores are closed like macOS's
// Keychain and browsers, on every system, and the macOS entries stay.
func TestKeyringsBrowsersAndPasswordStoresAreProtected(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, ".local", "share", "freeloop"))
	for _, rel := range []string{
		".local/share/keyrings/login.keyring", ".local/share/kwalletd/kdewallet.kwl",
		".mozilla/firefox/abc.default/cookies.sqlite", ".config/google-chrome/Default/Login Data",
		".config/chromium/Default/Cookies", ".config/BraveSoftware/Brave-Browser/Default/Cookies",
		".config/microsoft-edge/Default/Cookies", ".config/vivaldi/Default/Cookies", ".config/opera/Cookies",
		"snap/firefox/common/.mozilla/firefox/x/key4.db", ".var/app/org.mozilla.firefox/.mozilla/firefox/x/key4.db",
		".pki/nssdb/key4.db", ".password-store/github.com.gpg", ".config/Bitwarden/data.json",
		".config/KeePassXC/keepassxc.ini", ".local/share/freeloop/config.yaml",
		// What was there before.
		"Library/Keychains/login.keychain-db", "Library/Application Support/Google/Chrome/Default/Cookies", ".ssh/id_ed25519",
	} {
		for _, path := range []string{filepath.Join(home, rel), filepath.Dir(filepath.Join(home, rel))} {
			if p.readRefusal(path) == "" {
				t.Errorf("%s is readable", path)
			}
		}
		if p.writeRefusal(filepath.Join(home, "repo"), filepath.Join(home, rel)) == "" {
			t.Errorf("%s is writable", rel)
		}
	}
	// Neighbours stay open.
	for _, rel := range []string{".config/nvim/init.lua", ".local/share/nvim/x", ".config/google-chrome-notes", ".mozilla-notes/a"} {
		if why := p.readRefusal(filepath.Join(home, rel)); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
}

// Windows' credential stores, DPAPI's keys and the browsers under AppData are closed
// on every system, and so are the ones under a moved %APPDATA% (NFR-XPLAT-003).
func TestWindowsSecretsAreProtected(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, "AppData", "Roaming", "freeloop"))
	for _, rel := range []string{
		"AppData/Roaming/Microsoft/Credentials/ABC", "AppData/Local/Microsoft/Credentials/ABC",
		"AppData/Roaming/Microsoft/Protect/S-1-5-21/key", "AppData/Roaming/Microsoft/Vault/x",
		"AppData/Roaming/Microsoft/Crypto/RSA/key", "AppData/Roaming/Mozilla/Firefox/Profiles/x/key4.db",
		"AppData/Local/Google/Chrome/User Data/Default/Cookies", "AppData/Local/Microsoft/Edge/User Data/Default/Login Data",
		"AppData/Local/BraveSoftware/Brave-Browser/User Data/Default/Cookies", "AppData/Roaming/GitHub CLI/hosts.yml",
		"AppData/Roaming/gcloud/credentials.db", "AppData/Roaming/freeloop/config.yaml", "_netrc",
		".ssh/id_ed25519", ".aws/credentials", ".azure/msal_token_cache.json", ".docker/config.json", ".git-credentials", ".npmrc",
	} {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if p.readRefusal(path) == "" {
			t.Errorf("%s is readable", rel)
		}
		if p.writeRefusal(filepath.Join(home, "repo"), path) == "" {
			t.Errorf("%s is writable", rel)
		}
	}
	for _, rel := range []string{"AppData/Local/Programs/Microsoft VS Code/Code.exe", "AppData/Roaming/npm/node_modules/x", "AppData/Local/Temp/x"} {
		if why := p.readRefusal(filepath.Join(home, filepath.FromSlash(rel))); why != "" {
			t.Errorf("%s refused: %s", rel, why)
		}
	}
	moved := filepath.Join(home, "elsewhere")
	got := windowsSecrets(home, filepath.Join(moved, "Roaming"), filepath.Join(moved, "Local"))
	for _, want := range []string{filepath.Join(moved, "Roaming", "Microsoft", "Protect"), filepath.Join(moved, "Local", "Google", "Chrome"), filepath.Join(home, "AppData", "Roaming", "Microsoft", "Protect")} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not protected", want)
		}
	}
	if n := len(windowsSecrets(home, "relative", "")); n != len(windowsSecrets(home, "", "")) {
		t.Error("a relative %APPDATA% was taken")
	}
}

// HFS+ passes over some default-ignorable code points in a name, so .g<U+200C>it is
// .git there, as git's core.protectHFS reads it. The guard read it as another name and
// let a write into .git through (issue 079).
func TestIgnoredCodePointsDoNotHideProtectedNames(t *testing.T) {
	home := environment.Canonical(t.TempDir())
	p := NewPolicy(home, filepath.Join(home, "data"))
	root := filepath.Join(home, "repo")
	zwnj, zwj, bom := string(rune(0x200C)), string(rune(0x200D)), string(rune(0xFEFF))
	for _, rel := range []string{".g" + zwnj + "it/config", "sub/.gi" + bom + "t/hooks/pre-commit", ".free" + zwj + "loop/hooks.yaml"} {
		if p.writeRefusal(root, filepath.Join(root, filepath.FromSlash(rel))) == "" {
			t.Errorf("%q is writable", rel)
		}
	}
	if p.readRefusal(filepath.Join(home, ".s"+zwnj+"sh", "id_ed25519")) == "" {
		t.Error("~/.s<U+200C>sh is readable")
	}
	if !p.actedOn(root, []string{filepath.Join(root, "sub", ".env"+zwj+"rc")}) {
		t.Error("sub/.env<U+200D>rc is not acted on later")
	}
	r := Rule{Effect: Deny, Tool: "read_file", Path: "secrets/**"}
	if !r.Matches(Request{Tool: "read_file", Kind: KindRead, Paths: []string{filepath.Join(root, "sec"+bom+"rets", "key")}}, root) {
		t.Error("a deny rule missed sec<U+FEFF>rets/key")
	}
}
