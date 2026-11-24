package environment

import "testing"

// Each pair opens one file on APFS.
func TestFoldPathFollowsAPFS(t *testing.T) {
	for _, pair := range [][2]string{
		{"passwords.txt", "paßwords.txt"}, {"PASSWORDS", "paẞwords"}, {".ssh", ".ßh"}, {".config", ".conﬁg"}, {"office", "oﬃce"}, {"aster", "aﬅer"},
		{".ssh", ".ſsh"}, {"key", "Key"}, {"café", "café"}, {".GIT", ".git"},
	} {
		if FoldPath(pair[0]) != FoldPath(pair[1]) {
			t.Errorf("%q and %q differ: %q, %q", pair[0], pair[1], FoldPath(pair[0]), FoldPath(pair[1]))
		}
	}
	if FoldPath("a.txt") == FoldPath("b.txt") {
		t.Error("everything folds to one name")
	}
}

// HFS+ ignores some default-ignorable code points in a name, so .g<U+200C>it is .git
// there; git's core.protectHFS skips the same ones (issue 079).
func TestFoldPathDropsWhatHFSIgnores(t *testing.T) {
	ignored := []rune{0x200C, 0x200D, 0x200E, 0x200F, 0x202A, 0x202B, 0x202C, 0x202D, 0x202E, 0x206A, 0x206B, 0x206C, 0x206D, 0x206E, 0x206F, 0xFEFF}
	for _, r := range ignored {
		if got := FoldPath(".g" + string(r) + "it/config"); got != ".git/config" {
			t.Errorf("U+%04X: %q", r, got)
		}
	}
	zwnj, zwj, bom := string(rune(0x200C)), string(rune(0x200D)), string(rune(0xFEFF))
	if got := FoldPath(bom + ".GIT" + zwj + "/con" + zwnj + "fig"); got != ".git/config" {
		t.Errorf("several: %q", got)
	}
	// An accent after an ignored joiner is the accent of the letter before it.
	if FoldPath("cafe"+zwj+"\u0301") != FoldPath("caf\u00e9") {
		t.Error("a joiner kept an accent from its letter")
	}
	// What HFS+ does not ignore stays: a zero-width space or an emoji's variation
	// selector makes another name.
	for _, r := range []rune{0x200B, 0xFE0F, 0x00AD} {
		if FoldPath(".g"+string(r)+"it") == ".git" {
			t.Errorf("U+%04X is dropped", r)
		}
	}
}
