package environment

import (
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
