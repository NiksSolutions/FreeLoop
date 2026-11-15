//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Other programs' Unix sockets (ENV-009). Mounting the root read-only does not
// stop a connect(): a socket a program listens on anywhere the sandbox shows
// answers, and some answer anyone, LXD's with root on this computer. Seatbelt
// refuses every socket outside the project; bubblewrap has no rule for that, so
// what it shows of such a socket is replaced when a command starts:
//
//   - the folders where programs keep theirs (/tmp, /run, /var/tmp, home) are
//     emptied, and so are those of daemons that answer whoever reaches them
//     (socketDirs), whether they are running yet or not;
//   - every other socket a program on this computer is bound to, as
//     /proc/net/unix lists them, is hidden by emptying its folder and mounting
//     back what else is in it, read-only. Hiding the socket alone would mean
//     mounting over it, which bubblewrap refuses, and the command then fails,
//     when the socket has gone by the time it starts.
//
// The project's own sockets, its caches' and temp files' stay: a dev server's.

// socketDirs hold sockets of daemons that give whoever connects power over this
// computer: LXD and Incus (root, to their group), the Nix daemon, and WSLg's X
// server and Wayland and audio sockets.
var socketDirs = []string{
	"/var/snap/lxd/common/lxd", "/var/lib/lxd", "/var/lib/incus",
	"/nix/var/nix/daemon-socket",
	"/mnt/wslg/.X11-unix", "/mnt/wslg/runtime-dir",
}

// emptiedDirs are the folders bubblewrapArgs replaces with empty ones.
var emptiedDirs = []string{"/tmp", "/run", "/var/run", "/var/tmp", "/dev", "/proc"}

// maxCoverEntries bounds how many entries of a socket's folder are mounted back;
// a bigger folder has its socket mounted over instead.
const maxCoverEntries = 1000

// unixLine is a line of /proc/net/unix with a path: Num, RefCount, Protocol,
// Flags, Type, St, Inode, Path. An abstract socket's path starts with @.
var unixLine = regexp.MustCompile(`^\S+: \S+ \S+ \S+ \S+ \S+ +\d+ (/.*)$`)

// hostSockets are the paths of the sockets programs on this computer are bound
// to, as they are now, without links.
func hostSockets() []string {
	b, err := os.ReadFile("/proc/net/unix")
	if err != nil {
		return nil
	}
	return listedSockets(string(b))
}

// listedSockets are the existing sockets /proc/net/unix's text names.
func listedSockets(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		m := unixLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		p, err := filepath.EvalSymlinks(m[1])
		if err != nil || seen[p] {
			continue
		}
		seen[p] = true
		if fi, err := os.Lstat(p); err == nil && fi.Mode().Type() == os.ModeSocket {
			out = append(out, p)
		}
	}
	return out
}

// socketCover is what hides sockets: bwrap's options, for the folder or the
// socket at path.
type socketCover struct {
	path string
	args []string
	// dir is an emptied folder, to be made read-only last.
	dir bool
}

// socketCovers hide sockets in the folders that hold them. A folder too big to
// mount back, or the root, has each socket mounted over instead.
func socketCovers(sockets []string) []socketCover {
	byDir := map[string][]string{}
	for _, s := range sockets {
		byDir[filepath.Dir(s)] = append(byDir[filepath.Dir(s)], s)
	}
	var out []socketCover
	for dir, socks := range byDir {
		entries, err := os.ReadDir(dir)
		if dir == "/" || err != nil || len(entries) > maxCoverEntries {
			for _, s := range socks {
				out = append(out, socketCover{path: s, args: []string{"--ro-bind", "/dev/null", s}})
			}
			continue
		}
		args := []string{"--tmpfs", dir}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			switch {
			case e.Type()&os.ModeSocket != 0:
			case e.Type()&os.ModeSymlink != 0:
				if target, err := os.Readlink(p); err == nil {
					args = append(args, "--symlink", target, p)
				}
			default:
				// -try: what is gone by the time the command starts is left out.
				args = append(args, "--ro-bind-try", p, p)
			}
		}
		out = append(out, socketCover{path: dir, args: args, dir: true})
	}
	slices.SortFunc(out, func(x, y socketCover) int {
		if d := depth(x.path) - depth(y.path); d != 0 {
			return d
		}
		return strings.Compare(x.path, y.path)
	})
	return out
}

// shownSockets are those of sockets a command confined by these folders would
// see and is not given: outside the folders it writes and those emptied, unless
// mounted back.
func shownSockets(sockets []string, home string, write, read []string) []string {
	in := func(p string, dirs []string) bool {
		return slices.ContainsFunc(dirs, func(d string) bool { return within(p, d) })
	}
	emptied := append(append(slices.Clone(emptiedDirs), socketDirs...), home)
	var out []string
	for _, s := range sockets {
		if in(s, write) || (in(s, emptied) && !in(s, read)) {
			continue
		}
		out = append(out, s)
	}
	return out
}
