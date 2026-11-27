package config

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// A font is a family name on this computer (GUI-025). The names go into a style
// sheet, in quotes, so only what family names are made of passes: quotes,
// semicolons, braces, parentheses and slashes cannot get in, so a value edited
// into the file can neither add a rule nor make the page fetch something.
var fontName = regexp.MustCompile(`^[\p{L}\p{N}](?:[\p{L}\p{N}\p{M} ._+-]*[\p{L}\p{N}\p{M}._+-])?$`)

const maxFontName = 64

// ValidFontName reports whether v can name a font. Empty is not a name: in
// the settings it means the app's own font.
func ValidFontName(v string) bool {
	return utf8.RuneCountInString(v) <= maxFontName && fontName.MatchString(v)
}

// validateFont clears a font that is not a family name and reports it.
func validateFont(field string, v *string) []Issue {
	if *v == "" || ValidFontName(*v) {
		return nil
	}
	issue := Issue{Field: field, Message: fmt.Sprintf("%q is not a font name; using the app's own font", *v)}
	*v = ""
	return []Issue{issue}
}

// ValidWeight reports whether w is a font weight the settings take: 100 to 900
// in steps of 100, as CSS names them. Zero, the app's own, is not a weight.
func ValidWeight(w int) bool {
	return w >= 100 && w <= 900 && w%100 == 0
}

// validateWeight clears a weight that is not one and reports it.
func validateWeight(field string, w *int) []Issue {
	if *w == 0 || ValidWeight(*w) {
		return nil
	}
	issue := Issue{Field: field, Message: fmt.Sprintf("%d is not a weight from 100 to 900 in steps of 100; using the app's own", *w)}
	*w = 0
	return []Issue{issue}
}
