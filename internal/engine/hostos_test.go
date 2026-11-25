package engine

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// NFR-XPLAT-003: what differs on Windows, tested on any system through the system's
// name.

func TestGitBashPaths(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\a\AppData\Local\freeloop\tools\rtk-1.1.0`: "/c/Users/a/AppData/Local/freeloop/tools/rtk-1.1.0",
		`D:\`:                "/d/",
		`\\host\share\a`:     "//host/share/a",
		"/already/unix":      "/already/unix",
		`relative\path\a.go`: "relative/path/a.go",
	} {
		if got := bashPath("windows", in); got != want {
			t.Errorf("bashPath(windows, %q) = %q, want %q", in, got, want)
		}
	}
	if got := bashPath("darwin", `/Users/a/b\c`); got != `/Users/a/b\c` {
		t.Errorf("bashPath(darwin) changed the path: %q", got)
	}
	for in, want := range map[string]string{
		"/c/Users/a/bin/rtk": `C:\Users\a\bin\rtk`,
		"/d":                 `D:\`,
		"//host/share/a":     `\\host\share\a`,
		"/usr/bin/rtk":       "/usr/bin/rtk",
	} {
		if got := windowsPath(in); got != want {
			t.Errorf("windowsPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocatedProgram(t *testing.T) {
	for _, c := range []struct{ goos, out, want string }{
		{"darwin", "/opt/homebrew/bin/rtk\n", "/opt/homebrew/bin/rtk"},
		{"linux", "welcome from .profile\n/usr/local/bin/rtk\n", "/usr/local/bin/rtk"},
		{"linux", "rtk\n", ""}, // an alias or a function
		{"windows", "C:\\Users\\a\\.cargo\\bin\\rtk.exe\r\n", `C:\Users\a\.cargo\bin\rtk.exe`},
		{"windows", "/c/Users/a/bin/rtk\n", `C:\Users\a\bin\rtk`},
		{"windows", `\\host\tools\rtk.exe`, `\\host\tools\rtk.exe`},
		{"windows", "rtk\n", ""},
		{"windows", "/usr/bin/rtk\n", ""},
	} {
		if got := locatedPath(c.goos, c.out); got != c.want {
			t.Errorf("locatedPath(%s, %q) = %q, want %q", c.goos, c.out, got, c.want)
		}
	}
	if got := locateCommand("darwin", "rtk"); got != "command -v 'rtk'" {
		t.Errorf("locateCommand(darwin) = %q", got)
	}
	if got := locateCommand("windows", "rtk"); !strings.Contains(got, "cygpath -w") || !strings.Contains(got, "command -v 'rtk'") {
		t.Errorf("locateCommand(windows) = %q", got)
	}
}

func TestProgramNamesAndFolders(t *testing.T) {
	if exeName("windows", "rtk") != "rtk.exe" || exeName("windows", "rtk.EXE") != "rtk.EXE" || exeName("linux", "rtk") != "rtk" {
		t.Error("exeName")
	}
	if got := neutralDirOn("windows", "D:"); got != `D:\` {
		t.Errorf("neutralDirOn(windows, D:) = %q", got)
	}
	if got := neutralDirOn("windows", ""); got != `C:\` {
		t.Errorf("neutralDirOn(windows, \"\") = %q", got)
	}
	if got := neutralDirOn("darwin", "C:"); got != "/" {
		t.Errorf("neutralDirOn(darwin) = %q", got)
	}
	if got := fileURIPath("C:/Users/a/x.go"); got != "/C:/Users/a/x.go" {
		t.Errorf("fileURIPath = %q", got)
	}
	if got := fileURIPath("/Users/a/x.go"); got != "/Users/a/x.go" {
		t.Errorf("fileURIPath = %q", got)
	}
	for _, c := range []struct{ goos, home, local, want string }{
		{"darwin", "/Users/a", "", "/Users/a/Library/Caches/Freeloop"},
		{"linux", "/home/a", "", "/home/a/.cache/freeloop"},
		{"windows", "/home/a", "/somewhere/Local", "/somewhere/Local/freeloop"},
		{"windows", "/home/a", "", "/home/a/AppData/Local/freeloop"},
	} {
		// The folder is joined with this system's separator, as the only real
		// call, for this system, needs.
		if got, want := appCacheBase(c.goos, c.home, c.local), filepath.FromSlash(c.want); got != want {
			t.Errorf("appCacheBase(%s) = %q, want %q", c.goos, got, want)
		}
	}
}

// OQ-044: Windows has no native sandbox; the form says so, and so does a session.
func TestNoSandboxOnWindows(t *testing.T) {
	if why := sandboxRefusal("windows", false); !strings.Contains(why, "Windows has no native sandbox") || !strings.Contains(why, "Docker") {
		t.Errorf("sandboxRefusal(windows) = %q", why)
	}
	if why := sandboxRefusal("windows", true); why != "" {
		t.Errorf("a Windows confiner was refused: %q", why)
	}
	if why := sandboxRefusal("linux", false); why != "" {
		t.Errorf("sandboxRefusal(linux) = %q", why)
	}
	if !strings.Contains(noSandboxReason("linux"), "no sandbox") {
		t.Error(noSandboxReason("linux"))
	}
}

func TestGitOnWindows(t *testing.T) {
	win := safeGitOn("windows", `D:\Windows`)
	if !slices.Contains(win, `core.hooksPath=D:\Windows\System32\freeloop-no-hooks`) || slices.Contains(win, "core.hooksPath=/dev/null") {
		t.Errorf("safeGitOn(windows) = %q", win)
	}
	if got := safeGitOn("windows", ""); !slices.Contains(got, `core.hooksPath=C:\Windows\System32\freeloop-no-hooks`) {
		t.Errorf("safeGitOn(windows, \"\") = %q", got)
	}
	if got := safeGitOn("darwin", `C:\Windows`); !slices.Equal(got, safeGit) {
		t.Errorf("safeGitOn(darwin) = %q", got)
	}
	// OQ-046: (a) reads the system config, (b) does not and passes what it needs.
	narrow := systemGit{Narrow: true, AutoCRLF: "true", Helpers: []string{"manager"}}
	if _, ok := gitEnvOn("windows", systemGit{}, nil)["GIT_CONFIG_NOSYSTEM"]; ok {
		t.Error("(a): Windows' git does not read its system config")
	}
	if gitEnvOn("windows", narrow, nil)["GIT_CONFIG_NOSYSTEM"] != "1" {
		t.Error("(b): Windows' git reads all of its system config")
	}
	if gitEnvOn("linux", systemGit{}, map[string]string{"A": "b"})["GIT_CONFIG_NOSYSTEM"] != "1" || gitEnvOn("linux", systemGit{}, map[string]string{"A": "b"})["A"] != "b" {
		t.Error("gitEnvOn(linux)")
	}

	// gh's git calls get the same, through the environment.
	pairs := [][2]string{{"credential.helper", "manager"}, {"core.autocrlf", "true"}}
	vars := pushEnvOn("windows", narrow, pairs)
	for k, v := range map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_COUNT": "2", "GIT_CONFIG_KEY_0": "credential.helper", "GIT_CONFIG_VALUE_0": "manager", "GIT_CONFIG_KEY_1": "core.autocrlf", "GIT_CONFIG_VALUE_1": "true", "GH_PROMPT_DISABLED": "1"} {
		if vars[k] != v {
			t.Errorf("pushEnvOn(windows, (b))[%s] = %q, want %q", k, vars[k], v)
		}
	}
	vars = pushEnvOn("windows", systemGit{}, nil)
	if _, ok := vars["GIT_CONFIG_NOSYSTEM"]; ok || vars["GIT_CONFIG_COUNT"] != "" {
		t.Errorf("pushEnvOn(windows, (a)) = %v", vars)
	}
	if vars = pushEnvOn("darwin", systemGit{}, nil); vars["GIT_CONFIG_NOSYSTEM"] != "1" || vars["GIT_CONFIG_COUNT"] != "" {
		t.Errorf("pushEnvOn(darwin) = %v", vars)
	}
}

// OQ-046, decided 2026-11-25: from what `git config --system` reports, the app's git on
// Windows gets core.autocrlf and the credential helper when that is Git Credential
// Manager by its own path, and no other system setting (b). Any other helper, or none,
// and it reads the system config with hooks off, as built on 2026-11-08 (a).
func TestSystemGitConfigOnWindows(t *testing.T) {
	gcm := `C:/Program\ Files/Git/mingw64/bin/git-credential-manager.exe`
	own := `C:/Program\ Files\ \(x86\)/Git\ Credential\ Manager/git-credential-manager.exe`
	// What git prints with -z: each entry is its key, a newline and its value, then a
	// NUL; a key with no value at all has neither newline nor value.
	out := func(entries ...string) string {
		var b strings.Builder
		for _, e := range entries {
			b.WriteString(strings.Replace(e, "=", "\n", 1) + "\x00")
		}
		return b.String()
	}
	helper := func(v string) string { return "credential.helper=" + v }
	for _, c := range []struct {
		name     string
		out      string
		autocrlf string
		helpers  []string // nil for (a)
	}{
		{"Git for Windows", out("core.autocrlf=true", helper("manager")), "true", []string{"manager"}},
		{"autocrlf false", out("core.autocrlf=false", helper("manager")), "false", []string{"manager"}},
		{"autocrlf input", out("core.autocrlf=input", helper("manager")), "input", []string{"manager"}},
		{"no autocrlf", out(helper("manager")), "", []string{"manager"}},
		{"autocrlf twice, the last wins", out("core.autocrlf=true", "core.autocrlf=false", helper("manager")), "false", []string{"manager"}},
		{"autocrlf git would not read", out("core.autocrlf=maybe", helper("manager")), "", []string{"manager"}},
		{"manager-core", out("core.autocrlf=true", helper("manager-core")), "true", []string{"manager-core"}},
		{"GCM by its path in Git", out("core.autocrlf=true", helper(gcm)), "true", []string{gcm}},
		{"GCM by its path in its own folder", out(helper(own)), "", []string{own}},
		{"GCM without .exe", out(helper("C:/Git/mingw64/bin/git-credential-manager")), "", []string{"C:/Git/mingw64/bin/git-credential-manager"}},
		{"GCM Core by its path", out(helper("D:/Git/mingw64/libexec/git-core/git-credential-manager-core.exe")), "", []string{"D:/Git/mingw64/libexec/git-core/git-credential-manager-core.exe"}},
		{"an empty helper first", out(helper(""), helper("manager")), "", []string{"manager"}},
		{"another helper cleared before GCM", out(helper("wincred"), helper(""), helper("manager")), "", []string{"manager"}},
		{"GCM twice", out(helper("manager"), helper(gcm)), "", []string{"manager", gcm}},

		{"no system config", "", "", nil},
		{"autocrlf and no helper", out("core.autocrlf=true"), "", nil},
		{"wincred", out("core.autocrlf=true", helper("wincred")), "", nil},
		{"store", out(helper("store")), "", nil},
		{"Manager in capitals", out(helper("Manager")), "", nil},
		{"another program by its path", out(helper("C:/Tools/helper.exe")), "", nil},
		{"a shell snippet", out(helper(`!sh -c 'echo password=x'`)), "", nil},
		{"GCM as a shell snippet", out(helper("!" + gcm)), "", nil},
		{"GCM with an argument", out(helper(gcm + " --no-ui")), "", nil},
		{"GCM and a second command", out(helper(gcm + ";calc.exe")), "", nil},
		{"GCM by a path the shell would split", out(helper("C:/Program Files/Git/mingw64/bin/git-credential-manager.exe")), "", nil},
		{"GCM by a path with backslashes", out(helper(`C:\Program Files\Git\mingw64\bin\git-credential-manager.exe`)), "", nil},
		{"GCM by a path with a variable", out(helper("C:/$GIT/git-credential-manager.exe")), "", nil},
		{"GCM by a relative path", out(helper("bin/git-credential-manager.exe")), "", nil},
		{"a program named after GCM", out(helper("C:/Tools/not-git-credential-manager.exe")), "", nil},
		{"GCM and another helper", out(helper("manager"), helper("wincred")), "", nil},
		{"GCM, then cleared", out(helper("manager"), helper("")), "", nil},
		{"a helper for one address", out(helper("manager"), "credential.https://dev.azure.com.helper=manager"), "", nil},
		{"a helper without a value", out("credential.helper", helper("manager")), "", nil},
	} {
		got := systemGitFrom(c.out)
		want := systemGit{Narrow: c.helpers != nil, AutoCRLF: c.autocrlf, Helpers: c.helpers}
		if got.Narrow != want.Narrow || got.AutoCRLF != want.AutoCRLF || !slices.Equal(got.Helpers, want.Helpers) {
			t.Errorf("%s: systemGitFrom(%q) = %+v, want %+v", c.name, c.out, got, want)
		}
	}
}

func TestInstallHints(t *testing.T) {
	if !strings.Contains(nodeHint("windows"), "winget") || !strings.Contains(nodeHint("darwin"), "brew") || strings.Contains(nodeHint("linux"), "brew") {
		t.Errorf("nodeHint: %q %q %q", nodeHint("windows"), nodeHint("darwin"), nodeHint("linux"))
	}
	if !strings.Contains(uvHint("windows"), "winget") || !strings.Contains(uvHint("darwin"), "brew") {
		t.Error("uvHint")
	}
}

func TestAnonymizeWindowsHome(t *testing.T) {
	in := `open C:\Users\Niklas\proj\a.go; git: C:/Users/Niklas/proj; bash: /c/Users/Niklas/proj; json: "C:\\Users\\Niklas\\x"; lower c:\users\niklas\y; other C:\Users\Nik`
	got := anonymizeHome("windows", `C:\Users\Niklas`, in)
	want := `open ~\proj\a.go; git: ~/proj; bash: ~/proj; json: "~\\x"; lower ~\y; other C:\Users\Nik`
	if got != want {
		t.Errorf("anonymizeHome =\n%s\nwant\n%s", got, want)
	}
	if got := anonymizeHome("darwin", "/Users/a/", "/Users/a/x"); got != "~/x" {
		t.Errorf("anonymizeHome(darwin) = %q", got)
	}
}

// Windows drops the dots and spaces a name ends with, so .freeloop./hooks.yaml is
// .freeloop\hooks.yaml there.
func TestActedOnFileWindowsSpellings(t *testing.T) {
	for _, name := range []string{".freeloop./hooks.yaml", ".gitmodules. ", `.vscode\settings.json`, ".github. /workflows/ci.yml"} {
		if !actedOnFile(name) {
			t.Errorf("%s is not acted on", name)
		}
	}
	if actedOnFile("src/main.go") {
		t.Error("src/main.go is acted on")
	}
}

// RTK's Windows build is a .zip holding rtk.exe.
func TestProgramFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{"LICENSE": "license", "rtk.exe": "MZ program"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	got, err := programFromArchive(buf.Bytes(), "rtk.exe")
	if err != nil || string(got) != "MZ program" {
		t.Fatalf("programFromArchive = %q, %v", got, err)
	}
	if _, err := programFromArchive(buf.Bytes(), "rtk"); err == nil {
		t.Error("a zip without the program was accepted")
	}
}

// An engine opened on another home folder keeps its caches in that home, on
// Windows too, where %LOCALAPPDATA% is the real user's: tests and their downloads
// must not share the user's caches, or each other's.
func TestCachesFollowTheHome(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	e := openTest(t, t.TempDir())
	defer e.Close()
	home := environment.Canonical(e.home)
	if got := e.appCache("updates"); !strings.HasPrefix(got, home+string(filepath.Separator)) {
		t.Errorf("appCache = %q, not in the home folder %q", got, home)
	}
}

// OQ-046 (b) keeps Git for Windows' TLS settings: its system config names the CA
// bundle and the TLS backend its curl needs, and without them an https push fails.
// They are a file and a name, not a program, so they are passed; a backend git would
// not read is not, and they do not change the choice between (a) and (b).
func TestSystemGitKeepsTLS(t *testing.T) {
	bundle := "C:/Program Files/Git/mingw64/etc/ssl/certs/ca-bundle.crt"
	out := "core.autocrlf\ntrue\x00http.sslbackend\nopenssl\x00http.sslcainfo\n" + bundle + "\x00credential.helper\nmanager\x00"
	got := systemGitFrom(out)
	want := [][2]string{{"http.sslbackend", "openssl"}, {"http.sslcainfo", bundle}}
	if !got.Narrow || !slices.Equal(got.TLS, want) {
		t.Errorf("Git for Windows' own: %+v", got)
	}
	got = systemGitFrom("http.sslbackend\nschannel\x00http.sslbackend\nmagic\x00http.sslcainfo\n\x00credential.helper\nmanager\x00")
	if !got.Narrow || len(got.TLS) != 0 {
		t.Errorf("a backend git does not know, an empty bundle: %+v", got)
	}
	if got = systemGitFrom("http.sslcainfo\n" + bundle + "\x00credential.helper\nwincred\x00"); got.Narrow {
		t.Errorf("another helper still means (a): %+v", got)
	}
}
