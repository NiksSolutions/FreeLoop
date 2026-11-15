//go:build !windows

package environment

// farPaths: elsewhere //server/share is a plain path of this computer's own disk.
const farPaths = false

// dosNames: elsewhere NUL, a trailing dot and a colon are parts of a plain name.
const dosNames = false
