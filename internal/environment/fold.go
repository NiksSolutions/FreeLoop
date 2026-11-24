package environment

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// FoldPath spells a path so that two names macOS treats as one file compare
// equal. APFS ignores case and Unicode normalization, and folds fully: long s
// (U+017F) is s, the Kelvin sign is K, sharp s is ss, a ligature is its letters,
// and an accent can be one code point or two. strings.ToLower knows none of
// that, so a rule on secrets/ missed secret\u017f/, which opens the same folder.
// HFS+ also passes over the default-ignorable code points hfsIgnorable lists, so
// .g\u200cit is .git there; they are dropped first, as git's core.protectHFS drops
// them, so an accent after one is still its letter's. On a case-sensitive volume,
// or one that keeps those code points, this errs towards refusing.
func FoldPath(p string) string {
	p = strings.Map(func(r rune) rune {
		if hfsIgnorable(r) {
			return -1
		}
		return r
	}, p)
	// A Caser keeps state, so each call has its own.
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(p)))
}

// hfsIgnorable reports a code point HFS+ passes over when it compares names: the
// joiners, the direction marks and controls, the deprecated format characters and
// the byte order mark (Apple's TN1150, and git's next_hfs_char). Other default-
// ignorable code points, such as a zero-width space or a variation selector, are
// part of the name on every file system, and stay.
func hfsIgnorable(r rune) bool {
	return r >= 0x200C && r <= 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x206A && r <= 0x206F || r == 0xFEFF
}
