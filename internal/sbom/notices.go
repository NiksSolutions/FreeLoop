package sbom

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime/debug"
	"slices"
	"strings"
)

// Where the licenses of what is in a build are. All come from the machine that
// built it, which holds every package the build used and the Go it used.
type NoticeSources struct {
	// ModCache is `go env GOMODCACHE`, NodeModules the frontend's node_modules.
	ModCache, NodeModules fs.FS
	// GoRoot is `go env GOROOT`, whose LICENSE covers Go's runtime and standard
	// library, which every binary carries. It comes from the toolchain rather than
	// the repository, so it follows the toolchain.
	GoRoot fs.FS
	// Natives are the native programs the app carries, with their license files
	// read from NativeRoot.
	Natives    []Native
	NativeRoot fs.FS
}

// fallbackLicenses holds, as licenses/<name>@<version>/, the license text of a
// package that ships without one, with SOURCE saying where the text came from.
// Each version has its own: whoever moves the package to another checks that its
// license is still the same.
//
//go:embed licenses
var fallbackLicenses embed.FS

// carried says what a system's packages carry that neither the binary's build
// information nor the lockfile lists: what the packaging takes from the build
// machine or from Wails (build/linux/Taskfile.yml's appimage task,
// build/windows/Taskfile.yml's package:files). Change one with the other.
var carried = map[string]string{
	"linux": `The AppImage also carries GTK 3, WebKitGTK 4.1 and the libraries they use,
copied from the system it was built on, each under its own license; the
copyright file of each library's package is in the AppImage, under
usr/share/doc. The .deb carries none of them: it uses the system's.`,
	"windows": `The installer also carries Microsoft's WebView2 bootstrapper
(MicrosoftEdgeWebview2Setup.exe, as the Wails CLI the build is pinned to
provides it), under Microsoft's terms, and runs it only when the Microsoft Edge
WebView2 Runtime is missing, to download and install the runtime from
Microsoft. The portable zip does not carry it.`,
}

// Notices writes the license and notice files of everything in a build, the same
// things Build lists, for shipping beside the app: Go's own first, then every
// package's, then the native programs'. A package with no such file takes the text
// in licenses/ for its name and version; with neither it is an error, which names
// every such package, since the text cannot say what is not there. Nothing
// depends on when it runs.
func Notices(app App, info *debug.BuildInfo, packageLock []byte, src NoticeSources) ([]byte, error) {
	if info == nil {
		return nil, errors.New("no build information")
	}
	var lock lockFile
	if err := json.Unmarshal(packageLock, &lock); err != nil {
		return nil, fmt.Errorf("package-lock.json: %w", err)
	}
	if src.GoRoot == nil {
		return nil, errors.New("no Go root to read Go's license from (go env GOROOT)")
	}
	goLicense, err := fs.ReadFile(src.GoRoot, "LICENSE")
	if err != nil {
		return nil, fmt.Errorf("Go's license, LICENSE in the Go root: %w", err)
	}
	type pkg struct {
		name, version string
		root          fs.FS
		dir           string
	}
	var pkgs []pkg
	for _, d := range info.Deps {
		if d.Replace != nil {
			d = d.Replace
		}
		pkgs = append(pkgs, pkg{d.Path, d.Version, src.ModCache, escapeModule(d.Path) + "@" + escapeModule(d.Version)})
	}
	for key, p := range lock.Packages {
		if !strings.HasPrefix(key, "node_modules/") || p.Dev || p.DevOpt || p.Link {
			continue
		}
		name := key[strings.LastIndex(key, "node_modules/")+len("node_modules/"):]
		// A nested copy is under its parent, not at the top.
		pkgs = append(pkgs, pkg{name, p.Version, src.NodeModules, strings.TrimPrefix(key, "node_modules/")})
	}
	slices.SortFunc(pkgs, func(a, b pkg) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return strings.Compare(a.version, b.version)
	})
	pkgs = slices.CompactFunc(pkgs, func(a, b pkg) bool { return a.name == b.name && a.version == b.version })

	var out bytes.Buffer
	var missing []string
	fmt.Fprintf(&out, "%s %s includes the following software, under these licenses.\n", app.Name, app.Version)
	if text := carried[setting(info, "GOOS")]; text != "" {
		out.WriteString("\n" + text + "\n")
	}
	heading(&out, "Go standard library and runtime", strings.TrimPrefix(info.GoVersion, "go"))
	out.WriteString("\n")
	out.Write(bytes.TrimSpace(goLicense))
	out.WriteString("\n")
	for _, p := range pkgs {
		root, dir, source := p.root, p.dir, ""
		files, err := licenseFiles(root, dir)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			root, dir = fallbackLicenses, "licenses/"+p.name+"@"+p.version
			if files, err = licenseFiles(root, dir); err != nil {
				return nil, err
			}
			if len(files) == 0 {
				missing = append(missing, p.name+" "+p.version)
				continue
			}
			b, err := fs.ReadFile(root, dir+"/SOURCE")
			if err != nil {
				return nil, fmt.Errorf("%s: where its license text came from: %w", dir, err)
			}
			source = strings.Join(strings.Fields(string(b)), " ")
		}
		heading(&out, p.name, p.version)
		if source != "" {
			out.WriteString("\nIt ships no license file; the text below is " + source + ".\n")
		}
		for _, f := range files {
			b, err := fs.ReadFile(root, path.Join(dir, f))
			if err != nil {
				return nil, err
			}
			out.WriteString("\n")
			out.Write(bytes.TrimSpace(b))
			out.WriteString("\n")
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no license file for %s; check its license and add the text as internal/sbom/licenses/<name>@<version>/, with a SOURCE saying where it came from", strings.Join(missing, ", "))
	}
	for _, n := range src.Natives {
		text, err := n.noticeOf(src.NativeRoot)
		if err != nil {
			return nil, err
		}
		heading(&out, n.Name, n.Version)
		out.WriteString("\n" + text + "\n")
	}
	return out.Bytes(), nil
}

// setting is one of a binary's build settings, such as GOOS.
func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

func heading(out *bytes.Buffer, name, version string) {
	fmt.Fprintf(out, "\n\n================================================================\n%s %s\n================================================================\n", name, version)
}

// licenseFiles names a package's license and notice files, in order.
func licenseFiles(root fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(root, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := strings.ToUpper(e.Name())
		if e.IsDir() {
			continue
		}
		for _, start := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE", "UNLICENSE"} {
			if strings.HasPrefix(name, start) {
				out = append(out, e.Name())
				break
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

// escapeModule writes a module path or version as the module cache does: an upper
// case letter is ! and the letter in lower case.
func escapeModule(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			b.WriteByte('!')
			c += 'a' - 'A'
		}
		b.WriteRune(c)
	}
	return b.String()
}
