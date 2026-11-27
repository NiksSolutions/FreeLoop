//go:build linux

package linux

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"slices"
	"strconv"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/environment"
)

// bwrapPaths are where bubblewrap is looked for: absolute, since a PATH the agent
// can influence must not choose what enforces the sandbox.
var bwrapPaths = []string{"/usr/bin/bwrap", "/bin/bwrap", "/usr/local/bin/bwrap"}

// maxPatternWalk bounds how many entries the sandbox looks through for what its path
// patterns match when a command starts: nested repositories, instruction files, what a
// deny rule names. A project bigger than that fails closed: its commands do not run.
// Only folders a pattern can match in are looked through (walkStarts).
var maxPatternWalk = 200_000

// Bubblewrap confines commands with bubblewrap on Linux (ENV-009, OQ-037), as Seatbelt
// does on macOS. Seatbelt allows by default and denies; bubblewrap builds what the
// command sees out of mounts, so the guarantees are kept another way:
//
//   - other programs: a private process namespace, so none can be seen, signalled or
//     read, and a new terminal session, so none can be fed input;
//   - files: the root read-only; the project, its caches and temp files writable;
//     the home folder emptied, with what may be read mounted back;
//   - sockets: /tmp, /var/tmp, /run and the home folder are fresh, so Docker's, the
//     SSH agent's, D-Bus's and the keyring's sockets are out of reach, and any other
//     program's socket is hidden too (sockets.go);
//   - network: always a namespace of its own, so the host's abstract sockets are out
//     of reach: none, or the sandbox's own loopback, or with all of it pasta's full
//     network. network says how each reaches this computer's loopback.
//
// What it cannot do is protect a path that does not exist when the command starts:
// only existing paths can be mounted read-only.
type Bubblewrap struct {
	// noPasta leaves pasta out even where it is installed, for tests of the
	// sandbox without it.
	noPasta bool
}

// Confine wraps argv in bwrap for c. An error means the command must not run.
func (b Bubblewrap) Confine(c environment.Confinement, argv []string) ([]string, error) {
	bwrap := firstFile(bwrapPaths)
	if bwrap == "" {
		return nil, errors.New("the sandbox needs bubblewrap: install the bubblewrap package")
	}
	n, err := b.network(c)
	if err != nil {
		return nil, err
	}
	args, err := bubblewrapArgs(c, n)
	if err != nil {
		return nil, err
	}
	out := append(append(append(slices.Clone(n.pasta), bwrap), args...), "--")
	return append(append(out, n.bridge...), argv...), nil
}

// BubblewrapArgs are bwrap's options for c, with only the sandbox's own loopback
// for localhost, in order: later mounts go on top.
func BubblewrapArgs(c environment.Confinement) ([]string, error) {
	return bubblewrapArgs(c, netPlan{})
}

func bubblewrapArgs(c environment.Confinement, n netPlan) ([]string, error) {
	if c.Home == "" || len(c.Write) == 0 {
		return nil, errors.New("a sandbox needs a home folder and a project")
	}
	a := []string{
		"--die-with-parent", "--new-session",
		"--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup-try",
	}
	// Never the host's network namespace, even for all of the network: that would
	// open the host's abstract sockets, X11's and D-Bus's among them, which no
	// mount can hide (security review, 2026-11-13).
	switch {
	case n.pasta != nil:
		// pasta's network namespace, whose loopback it serves. bwrap starts in it as
		// root of pasta's user namespace, which owns that network: as that, a command
		// could bring up pasta's interface and reach out through it. So the command
		// gets a user namespace of its own, as the user, with no capabilities.
		a = append(a, "--unshare-user", "--uid", strconv.Itoa(os.Getuid()), "--gid", strconv.Itoa(os.Getgid()), "--cap-drop", "ALL")
	default:
		a = append(a, "--unshare-net")
	}
	// Other programs' sockets the command would see (sockets.go). Those under a
	// folder mounted back go on top of it, below; the rest go first, so that what
	// is emptied or mounted next goes on top of them.
	a = append(a, "--ro-bind", "/", "/")
	var readOnly, readMounted []string
	type mount struct {
		path string
		args []string
	}
	var mounts, covers []mount
	for _, s := range socketCovers(shownSockets(hostSockets(), c.Home, append(slices.Clone(c.Write), c.Scratch...), c.Read)) {
		if s.dir {
			readOnly = append(readOnly, s.path)
		}
		if slices.ContainsFunc(c.Read, func(r string) bool { return within(s.path, r) }) {
			covers = append(covers, mount{s.path, s.args})
		} else {
			a = append(a, s.args...)
		}
	}

	a = append(a, "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp", "--tmpfs", "/run")
	// /var/run is usually a link to /run; a folder of its own gets emptied too.
	// /var/tmp is the other place for temp files and their sockets.
	for _, d := range []string{"/var/run", "/var/tmp"} {
		if isDir(d) {
			a = append(a, "--tmpfs", d)
		}
	}
	for _, d := range socketDirs {
		if isDir(d) {
			a = append(a, "--tmpfs", d)
			readOnly = append(readOnly, d)
		}
	}
	a = append(a, n.mounts...)
	a = append(a, "--tmpfs", c.Home)

	// What can be read and written, outer folders first so inner mounts go on top,
	// and a folder's sockets covered after it is mounted.
	for _, p := range c.Read {
		mounts = append(mounts, mount{p, []string{"--ro-bind", p, p}})
		readMounted = append(readMounted, p)
	}
	write := "--bind"
	if c.ReadOnly {
		write = "--ro-bind"
	}
	for _, p := range c.Write {
		mounts = append(mounts, mount{p, []string{write, p, p}})
		readMounted = append(readMounted, p)
	}
	for _, p := range c.Scratch {
		mounts = append(mounts, mount{p, []string{"--bind", p, p}})
		readMounted = append(readMounted, p)
	}
	mounts = append(mounts, covers...)
	slices.SortStableFunc(mounts, func(x, y mount) int { return depth(x.path) - depth(y.path) })
	for _, m := range mounts {
		if exists(m.path) {
			a = append(a, m.args...)
		}
	}

	walked, err := matching(c, append(append(append([]string{}, c.Write...), c.Scratch...), c.Read...))
	if err != nil {
		return nil, err
	}
	// A folder mounted on itself cannot be renamed or removed (EBUSY), and what is
	// inside stays writable: .git cannot be moved aside, edited and moved back.
	for _, p := range append(slices.Clone(c.NoMove), walked.noMove...) {
		if isDir(p) {
			a = append(a, "--bind", p, p)
		}
	}
	// Never written: read-only on top of the project.
	for _, p := range append(slices.Clone(c.Protected), walked.protected...) {
		if exists(p) {
			a = append(a, "--ro-bind", p, p)
		}
	}
	// Neither read nor written: an empty folder or file in its place. Under the home
	// folder only what was mounted back is there to hide.
	mounted := func(p string) bool {
		return slices.ContainsFunc(readMounted, func(m string) bool { return within(p, m) })
	}
	for _, p := range append(slices.Clone(c.NoAccess), walked.noAccess...) {
		if within(p, c.Home) && !mounted(p) {
			continue
		}
		switch {
		case isDir(p):
			a = append(a, "--tmpfs", p)
			readOnly = append(readOnly, p)
		case exists(p):
			a = append(a, "--ro-bind", "/dev/null", p)
		}
	}
	for _, p := range c.ReadAlways {
		if exists(p) {
			a = append(a, "--ro-bind", p, p)
		}
	}
	// Last, so the folders the mounts above needed could be made in them: nothing is
	// written to the emptied home folder or a hidden one, which would seem to work and
	// then be gone, and would let a command create the probe the self-test looks for.
	for _, p := range append(readOnly, c.Home) {
		a = append(a, "--remount-ro", p)
	}
	return a, nil
}

// within reports whether p is dir or inside it.
func within(p, dir string) bool {
	p, dir = filepath.Clean(p), filepath.Clean(dir)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// walkedPaths are the existing paths a deny rule's wildcard matches.
type walkedPaths struct{ protected, noMove, noAccess []string }

// matching finds what the patterns of c match under roots, when there are any.
func matching(c environment.Confinement, roots []string) (walkedPaths, error) {
	var w walkedPaths
	compile := func(res []string) ([]*regexp.Regexp, error) {
		var out []*regexp.Regexp
		for _, re := range res {
			r, err := regexp.Compile(re)
			if err != nil {
				return nil, fmt.Errorf("a path pattern does not compile: %w", err)
			}
			out = append(out, r)
		}
		return out, nil
	}
	protected, err := compile(c.ProtectedPatterns)
	if err != nil {
		return w, err
	}
	noMove, err := compile(c.NoMovePatterns)
	if err != nil {
		return w, err
	}
	noAccess, err := compile(c.NoAccessPatterns)
	if err != nil {
		return w, err
	}
	if len(protected)+len(noMove)+len(noAccess)+len(c.ProtectedNames) == 0 {
		return w, nil
	}
	any := func(rs []*regexp.Regexp, p string) bool {
		return slices.ContainsFunc(rs, func(r *regexp.Regexp) bool { return r.MatchString(p) })
	}
	named := func(names []string, name string) bool {
		return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
	}
	// One of ProtectedNames under Write and not inside installed code.
	kept := func(p string) bool {
		if !named(c.ProtectedNames, filepath.Base(p)) {
			return false
		}
		for _, root := range c.Write {
			if p != root && within(p, root) {
				rel, _ := filepath.Rel(root, filepath.Dir(p))
				return !slices.ContainsFunc(strings.Split(rel, string(filepath.Separator)), func(part string) bool { return named(c.Installed, part) })
			}
		}
		return false
	}
	var patterns []string
	for _, res := range [][]string{c.ProtectedPatterns, c.NoMovePatterns, c.NoAccessPatterns} {
		patterns = append(patterns, res...)
	}
	starts := walkStarts(patterns, roots)
	if len(c.ProtectedNames) > 0 {
		starts = outermost(append(starts, c.Write...))
	}
	seen, n := map[string]bool{}, 0
	for _, root := range starts {
		if !exists(root) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if n++; n > maxPatternWalk {
				return fmt.Errorf("the sandbox would have to look through more than %d files in %s for the paths it protects (nested repositories, instruction files, what a deny rule names), so commands do not run in it here", maxPatternWalk, root)
			}
			if seen[p] {
				return nil
			}
			seen[p] = true
			if any(noAccess, p) {
				w.noAccess = append(w.noAccess, p)
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if any(protected, p) || kept(p) {
				w.protected = append(w.protected, p)
			}
			if d.IsDir() && any(noMove, p) {
				w.noMove = append(w.noMove, p)
			}
			return nil
		})
		if err != nil {
			return w, err
		}
	}
	return w, nil
}

// walkStarts are the folders to look through for what patterns match under roots: for
// each pattern, the folder its matches all lie in (literalStart) where that is inside
// one of roots, or the root that folder holds; every root for a pattern that can match
// anywhere. A pattern for the project never makes a toolchain's files be looked through.
func walkStarts(patterns, roots []string) []string {
	var starts []string
	for _, re := range patterns {
		dir := literalStart(re)
		for _, root := range roots {
			switch {
			case dir == "" || within(root, dir):
				starts = append(starts, root)
			case within(dir, root):
				starts = append(starts, dir)
			}
		}
	}
	return outermost(starts)
}

// outermost drops the folders inside another of folders, which is looked through
// with it.
func outermost(folders []string) []string {
	folders = slices.Clone(folders)
	slices.Sort(folders)
	var out []string
	for _, f := range slices.Compact(folders) {
		if !slices.ContainsFunc(out, func(o string) bool { return within(f, o) }) {
			out = append(out, f)
		}
	}
	return out
}

// literalStart is the folder every match of re lies in: the plain text after its
// leading ^ up to the last slash in it, or "" for a pattern that can match anywhere,
// or whose start is not plain text.
func literalStart(re string) string {
	t, err := syntax.Parse(re, syntax.Perl)
	if err != nil {
		return ""
	}
	t = t.Simplify()
	if t.Op != syntax.OpConcat || len(t.Sub) == 0 || t.Sub[0].Op != syntax.OpBeginText {
		return ""
	}
	var lit strings.Builder
	for _, s := range t.Sub[1:] {
		if s.Op != syntax.OpLiteral || s.Flags&syntax.FoldCase != 0 {
			break
		}
		lit.WriteString(string(s.Rune))
	}
	text := lit.String()
	if i := strings.LastIndex(text, "/"); i > 0 {
		return text[:i]
	}
	return ""
}

func depth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func isDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir()
}
