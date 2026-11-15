package environment

import (
	"fmt"
	"path/filepath"
	"strings"
)

// farPath reports whether p, read as a Windows path, names a place on another
// computer or a device rather than a file of a drive: it starts with two slashes
// or backslashes, as \\server\share, //server/share, \\?\UNC\server\share,
// \\.\pipe\name and \\?\GLOBALROOT\... do. Looking at such a path, even only to
// resolve its links, opens a connection to that computer, which on Windows signs
// in as the user and so sends their NTLM hash. A drive in the verbatim spelling,
// \\?\C:\..., is a drive and not far. It is a pure function of the string, so it
// is tested on every system; farPaths says whether the system reads paths so.
func farPath(p string) bool {
	p = withoutVerbatimPrefix(p, true)
	sep := func(c byte) bool { return c == '\\' || c == '/' }
	return len(p) >= 2 && sep(p[0]) && sep(p[1])
}

// untouchable reports whether abs is not to be looked at on this system: a far
// path outside root, the project, which is approved as a whole. Within a project
// that is itself on a share the share is already connected to.
func untouchable(root, abs string) bool {
	return farPaths && farPath(abs) && !LexicallyWithin(root, abs)
}

// LexicallyWithin reports whether p is root or inside it as spelled, ignoring
// case as the permission policy does, without looking at the disk.
func LexicallyWithin(root, p string) bool {
	r, q := FoldPath(Unaliased(filepath.Clean(root))), FoldPath(Unaliased(filepath.Clean(p)))
	return q == r || strings.HasPrefix(q, strings.TrimSuffix(r, string(filepath.Separator))+string(filepath.Separator))
}

// dosDevice reports whether the last name of p, read as a Windows path, is one Windows
// opens as a device in whatever folder it is: CON, PRN, AUX, NUL, CLOCK$, CONIN$,
// CONOUT$, COM0 to COM9 and LPT0 to LPT9 (with ¹, ² and ³ too), in any case, with
// an extension, trailing spaces or a stream after a colon: C:\p\nul.txt is NUL, and
// reading C:\p\CON waits for someone to type. It is a pure function of the string, so
// it is tested on every system; dosNames says whether the system reads names so.
func dosDevice(p string) bool {
	name := winBase(p)
	if i := strings.IndexAny(name, ".:"); i >= 0 {
		name = name[:i]
	}
	name = strings.ToUpper(strings.TrimRight(name, " "))
	switch name {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	}
	if rest, ok := strings.CutPrefix(name, "COM"); ok {
		return portSuffix(rest)
	}
	if rest, ok := strings.CutPrefix(name, "LPT"); ok {
		return portSuffix(rest)
	}
	return false
}

// portSuffix is a serial or parallel port's number.
func portSuffix(s string) bool {
	return len(s) == 1 && s[0] >= '0' && s[0] <= '9' || s == "¹" || s == "²" || s == "³"
}

// dosAlias reports whether one name of a path, on Windows, opens something other than
// what it spells: a device (dosDevice), a stream of another file (a colon, as in
// .git::$INDEX_ALLOCATION, which is the folder .git), a name Windows trims (a trailing
// dot or space: ".git." is .git) or a short 8.3 name (GIT~1 can be .git). A check by
// name, such as the sync's on .git, would see one name and the file system another.
func dosAlias(name string) bool {
	if dosDevice(name) || strings.ContainsRune(name, ':') || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return true
	}
	for i := 0; i+1 < len(name); i++ {
		if name[i] == '~' && name[i+1] >= '0' && name[i+1] <= '9' {
			return true
		}
	}
	return false
}

// device refuses, on Windows, a path whose last name Windows opens as a device
// wherever it is (dosDevice): reading CON would wait for typing, and writing COM1
// or PRN would reach hardware. It is not a regular file.
func device(abs string) error {
	if dosNames && dosDevice(abs) {
		return fmt.Errorf("%s names a device on Windows, not a regular file", abs)
	}
	return nil
}
