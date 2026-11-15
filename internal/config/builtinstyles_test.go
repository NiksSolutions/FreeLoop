package config

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The tokens the color editor in Settings offers (COLOR_GROUPS in
// frontend/src/colors.ts): a built-in style sets each in both appearances, so
// choosing one leaves none of the app's own colors behind.
var editorTokens = []string{
	"bg-app", "bg", "surface", "surface-2", "input",
	"text", "muted", "faint", "border", "border-strong", "input-border",
	"accent", "accent-text", "link", "focus",
	"ok", "warn", "danger", "git",
}

// luminance is a #rrggbb color's relative luminance, as WCAG defines it.
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("%q is not #rrggbb", hex)
	}
	var l float64
	for i, w := range []float64{0.2126, 0.7152, 0.0722} {
		n, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatal(err)
		}
		c := float64(n) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		l += w * c
	}
	return l
}

func contrast(t *testing.T, a, b string) float64 {
	x, y := luminance(t, a), luminance(t, b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}

// GUI-053: the styles the app ships pass the checks a file's do, set every color
// the editor offers, and are easy to read: text at WCAG AA's 4.5:1 on the
// backgrounds it sits on (the pairs the editor warns about, and more), a field's
// edge and the status colors at 3:1.
func TestBuiltinStyles(t *testing.T) {
	styles := BuiltinStyles()
	if len(styles) < 4 {
		t.Fatalf("%d built-in styles", len(styles))
	}
	seen := map[string]bool{}
	for _, s := range styles {
		if !idPattern.MatchString(s.ID) || seen[s.ID] || !s.Builtin || s.Name == "" || s.Version != 1 {
			t.Errorf("style %+v", s)
		}
		seen[s.ID] = true
		if _, issues := cleanStyle(s.ID, styleFile{Version: 1, Name: s.Name, TextFont: s.TextFont, CodeFont: s.CodeFont, Light: s.Light, Dark: s.Dark}); len(issues) > 0 {
			t.Errorf("%s: %+v", s.ID, issues)
		}
		for mode, c := range map[string]map[string]string{"light": s.Light, "dark": s.Dark} {
			for _, token := range editorTokens {
				if c[token] == "" {
					t.Errorf("%s %s sets no %s", s.ID, mode, token)
				}
			}
			for _, p := range []struct {
				fg, bg string
				min    float64
			}{
				{"text", "bg", 4.5}, {"text", "bg-app", 4.5}, {"text", "surface", 4.5},
				{"muted", "bg", 4.5}, {"muted", "bg-app", 4.5}, {"faint", "bg", 4.5},
				{"accent-text", "accent", 4.5}, {"link", "bg", 4.5},
				{"input-border", "input", 3}, {"ok", "bg", 3}, {"warn", "bg", 3}, {"danger", "bg", 3}, {"git", "bg", 3},
			} {
				if c[p.fg] == "" || c[p.bg] == "" {
					continue
				}
				if r := contrast(t, c[p.fg], c[p.bg]); r < p.min {
					t.Errorf("%s %s: %s on %s is %.2f:1, under %.1f:1", s.ID, mode, p.fg, p.bg, r, p.min)
				}
			}
		}
	}
	// What a caller does with the list does not change the next one.
	styles[0].Name, styles[0].Light["bg"] = "changed", "#000000"
	if again := BuiltinStyles(); again[0].Name == "changed" || again[0].Light["bg"] == "#000000" {
		t.Error("the built-in styles share their maps with a caller")
	}
}

// A file with a built-in style's ID takes its place, as a preset's does, and the
// list stays sorted by name.
func TestStylesWithBuiltins(t *testing.T) {
	builtin := BuiltinStyles()
	mine := Style{ID: builtin[0].ID, Version: 1, Name: "Mine", Light: map[string]string{}, Dark: map[string]string{}}
	paper := Style{ID: "paper", Version: 1, Name: "Paper", Light: map[string]string{}, Dark: map[string]string{}}
	got := StylesWithBuiltins([]Style{paper, mine})
	if len(got) != len(builtin)+1 {
		t.Fatalf("%d styles", len(got))
	}
	i := slices.IndexFunc(got, func(s Style) bool { return s.ID == builtin[0].ID })
	if got[i].Name != "Mine" || got[i].Builtin {
		t.Errorf("the file did not replace the built-in: %+v", got[i])
	}
	if !slices.IsSortedFunc(got, func(a, b Style) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }) {
		t.Errorf("not sorted by name: %+v", got)
	}
	if !IsBuiltinStyle(builtin[1].ID) || IsBuiltinStyle("paper") {
		t.Error("IsBuiltinStyle")
	}
}
