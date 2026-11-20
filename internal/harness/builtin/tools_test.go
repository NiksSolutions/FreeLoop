package builtin

import (
	"strings"
	"testing"
)

// SKL-004: the skill's tag names its folder as it is spelled, a Windows path's single
// backslashes included, and a quote in a folder's name does not end the attribute.
func TestSkillTagNamesItsFolder(t *testing.T) {
	for dir, want := range map[string]string{
		`/Users/a/skills/pdf`:         `folder="/Users/a/skills/pdf"`,
		`C:\Users\a\skills\pdf`:       `folder="C:\Users\a\skills\pdf"`,
		`/Users/a/My "pdf" tools/pdf`: `folder="/Users/a/My &quot;pdf&quot; tools/pdf"`,
	} {
		got := skillTag("pdf", dir, "  The body.\n")
		if !strings.HasPrefix(got, `<skill name="pdf" `+want+">\nThe body.\n</skill>\n") {
			t.Errorf("%s: %q", dir, got)
		}
	}
}
