package sbom

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// noticeFolder is a stand-in build machine: a module cache, node_modules and a
// Go root under one folder.
func noticeFolder(t *testing.T, files map[string]string) (NoticeSources, string) {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"mod", "node", "goroot"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return NoticeSources{ModCache: os.DirFS(filepath.Join(root, "mod")), NodeModules: os.DirFS(filepath.Join(root, "node")), GoRoot: os.DirFS(filepath.Join(root, "goroot"))}, root
}

func TestNotices(t *testing.T) {
	src, _ := noticeFolder(t, map[string]string{
		"goroot/LICENSE": "go license",
		"mod/github.com/!azure/go@v1.0.0/LICENSE":  "azure license",
		"mod/example.com/noted@v2.0.0/LICENSE.txt": "noted license",
		"mod/example.com/noted@v2.0.0/NOTICE":      "noted notice",
		"mod/example.com/noted@v2.0.0/main.go":     "package main",
		"node/react/LICENSE":                       "react license",
		"node/a/node_modules/b/LICENSE.md":         "nested license",
		"node/devonly/LICENSE":                     "must not appear",
	})
	info := &debug.BuildInfo{GoVersion: "go1.27.4", Deps: []*debug.Module{
		{Path: "example.com/noted", Version: "v2.0.0"},
		{Path: "github.com/Azure/go", Version: "v1.0.0"},
		{Path: "example.com/old", Version: "v1.0.0", Replace: &debug.Module{Path: "example.com/noted", Version: "v2.0.0"}},
	}}
	lock := []byte(`{"packages":{
		"":{"version":"0.0.1"},
		"node_modules/react":{"version":"19.0.0"},
		"node_modules/a/node_modules/b":{"version":"1.0.0"},
		"node_modules/devonly":{"version":"1.0.0","dev":true}
	}}`)
	text, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lock, src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(text)
	for _, want := range []string{"azure license", "noted license", "noted notice", "react license", "nested license", "github.com/Azure/go v1.0.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "must not appear") || strings.Contains(got, "package main") {
		t.Errorf("too much in:\n%s", got)
	}
	if strings.Count(got, "noted license") != 1 {
		t.Errorf("a replaced module is listed twice:\n%s", got)
	}
	again, _ := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lock, src)
	if string(again) != got {
		t.Error("two runs differ")
	}
}

// Go's runtime and standard library are in every binary, under Go's license,
// which comes from the toolchain's own folder.
func TestNoticesCarryGosLicense(t *testing.T) {
	src, _ := noticeFolder(t, map[string]string{"goroot/LICENSE": "Copyright 2009 The Go Authors."})
	info := &debug.BuildInfo{GoVersion: "go1.27.4"}
	text, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, []byte(`{"packages":{}}`), src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(text)
	at := strings.Index(got, "Go standard library and runtime 1.27.4\n")
	if at < 0 || !strings.Contains(got[at:], "Copyright 2009 The Go Authors.") {
		t.Errorf("no heading for Go with its license after it:\n%s", got)
	}

	if _, err := Notices(App{Name: "Freeloop"}, info, []byte(`{"packages":{}}`), NoticeSources{ModCache: src.ModCache, NodeModules: src.NodeModules}); err == nil {
		t.Error("no Go root, and no error")
	}
	empty, _ := noticeFolder(t, nil)
	if _, err := Notices(App{Name: "Freeloop"}, info, []byte(`{"packages":{}}`), empty); err == nil || !strings.Contains(err.Error(), "LICENSE") {
		t.Errorf("a Go root without LICENSE: err = %v", err)
	}
}

// What ships without a license file stops the notices, unless the repository has
// its text: the notices cannot leave out what the app carries.
func TestNoticesFailOnAPackageWithoutALicense(t *testing.T) {
	src, _ := noticeFolder(t, map[string]string{
		"goroot/LICENSE":                     "go license",
		"mod/example.com/bare@v1.0.0/go.mod": "module bare",
	})
	info := &debug.BuildInfo{GoVersion: "go1.27.4", Deps: []*debug.Module{{Path: "example.com/bare", Version: "v1.0.0"}}}
	lock := []byte(`{"packages":{"node_modules/gone":{"version":"1.0.0"}}}`)
	text, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lock, src)
	if err == nil {
		t.Fatalf("no error, and:\n%s", text)
	}
	for _, want := range []string{"example.com/bare v1.0.0", "gone 1.0.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %s: %v", want, err)
		}
	}
}

func TestNoticesFallBackToTheRepositorysText(t *testing.T) {
	lockFor := func(version string) []byte {
		return []byte(`{"packages":{"node_modules/@wailsio/runtime":{"version":"` + version + `"}}}`)
	}
	info := &debug.BuildInfo{GoVersion: "go1.27.4"}
	src, _ := noticeFolder(t, map[string]string{"goroot/LICENSE": "go license", "node/@wailsio/runtime/package.json": "{}"})
	text, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lockFor("3.0.0"), src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(text)
	at := strings.Index(got, "@wailsio/runtime 3.0.0\n")
	if at < 0 {
		t.Fatalf("no heading for @wailsio/runtime:\n%s", got)
	}
	for _, want := range []string{"ships no license file", "github.com/wailsapp/wails", "MIT License", "Copyright (c) 2018-Present Lea Anthony"} {
		if !strings.Contains(got[at:], want) {
			t.Errorf("no %q under @wailsio/runtime:\n%s", want, got[at:])
		}
	}

	// Another version may be under another license: someone has to look again.
	if _, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lockFor("3.0.1"), src); err == nil || !strings.Contains(err.Error(), "@wailsio/runtime 3.0.1") {
		t.Errorf("a version with no text of its own: err = %v", err)
	}

	// A package's own file wins.
	own, _ := noticeFolder(t, map[string]string{"goroot/LICENSE": "go license", "node/@wailsio/runtime/LICENSE": "its own license"})
	text, err = Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, lockFor("3.0.0"), own)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(text); !strings.Contains(got, "its own license") || strings.Contains(got, "ships no license file") {
		t.Errorf("the repository's text was used over the package's own:\n%s", got)
	}
}

// Each text in licenses/ says where it came from, in SOURCE beside it.
func TestFallbackLicensesSayWhereTheyCameFrom(t *testing.T) {
	dirs := map[string]bool{}
	err := fs.WalkDir(fallbackLicenses, "licenses", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		dirs[path.Dir(name)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dirs["licenses/@wailsio/runtime@3.0.0"] {
		t.Errorf("no text for @wailsio/runtime 3.0.0 in %v", dirs)
	}
	for dir := range dirs {
		files, err := licenseFiles(fallbackLicenses, dir)
		if err != nil {
			t.Fatal(err)
		}
		source, err := fs.ReadFile(fallbackLicenses, dir+"/SOURCE")
		if len(files) == 0 || err != nil || strings.TrimSpace(string(source)) == "" {
			t.Errorf("%s: license files %v, SOURCE %q (%v)", dir, files, source, err)
		}
	}
}

// The packages carry things no license file covers: the AppImage the system's
// GTK and WebKitGTK, whose source is published beside it under the names
// build/linux/package.sh gives the archives, the Windows installer Microsoft's
// WebView2 bootstrapper.
func TestNoticesSayWhatThePackagesCarry(t *testing.T) {
	src, _ := noticeFolder(t, map[string]string{"goroot/LICENSE": "go license"})
	for goos, want := range map[string][]string{
		"linux": {"AppImage", "GTK 3", "WebKitGTK 4.1", "usr/share/doc", ".deb",
			"Freeloop-0.1.0-linux-amd64-sources.tar", "Freeloop-0.1.0-linux-arm64-sources.tar",
			"https://github.com/NiksSolutions/FreeLoop/releases/tag/v0.1.0"},
		"windows": {"installer", "WebView2 bootstrapper", "MicrosoftEdgeWebview2Setup.exe", "portable zip"},
		"darwin":  nil,
	} {
		info := &debug.BuildInfo{GoVersion: "go1.27.4", Settings: []debug.BuildSetting{{Key: "GOOS", Value: goos}}}
		text, err := Notices(App{Name: "Freeloop", Version: "0.1.0"}, info, []byte(`{"packages":{}}`), src)
		if err != nil {
			t.Fatal(err)
		}
		got := string(text)
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: no %q in:\n%s", goos, w, got)
			}
		}
		if goos == "darwin" && (strings.Contains(got, "GTK") || strings.Contains(got, "WebView2")) {
			t.Errorf("darwin: another system's packages in:\n%s", got)
		}
	}
}
