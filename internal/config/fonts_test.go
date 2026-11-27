package config

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestValidFontName(t *testing.T) {
	for _, v := range []string{"Menlo", "SF Mono", "JetBrains Mono NL", "Source Sans 3", "M+ 1m", "Noto_Sans.Mono-Bold", "ヒラギノ角ゴシック", "3270"} {
		if !ValidFontName(v) {
			t.Errorf("%q refused", v)
		}
	}
	for _, v := range []string{
		"", " Menlo", "Menlo ", "-Menlo", `Menlo"`, "Menlo'", "Menlo;", "Menlo, serif", "a{b}", `a\b`, "a/b", "a:b",
		`x"; } body { background: url(https://example.com/x) } a { b: "`, "url(https://example.com/x)", "var(--text)",
		"Menlo\nserif", "Menlo\x00", strings.Repeat("a", 65),
	} {
		if ValidFontName(v) {
			t.Errorf("%q passed", v)
		}
	}
}

func TestFontsSavedAndBadOnesRefused(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.UI.TextFont, c.UI.CodeFont = "Avenir Next", "JetBrains Mono"
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, issues, err := Load(dir)
	if err != nil || len(issues) != 0 || got.UI.TextFont != "Avenir Next" || got.UI.CodeFont != "JetBrains Mono" {
		t.Fatalf("Load = %+v, %v, %v", got.UI, issues, err)
	}
	c.UI.CodeFont = "x; } body { background: url(y) }"
	if err := Save(dir, c); err == nil {
		t.Error("a bad font was saved")
	}
	if got, _, _ := Load(dir); got.UI.CodeFont != "JetBrains Mono" {
		t.Errorf("the file changed: %+v", got.UI)
	}
}

func TestBadFontInTheFileIsReportedAndIgnored(t *testing.T) {
	got, issues, err := Load(write(t, "version: 1\nui:\n  theme: dark\n  textFont: 'a\"; } * { background: url(x) }'\n  codeFont: Menlo\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.UI.TextFont != "" || got.UI.CodeFont != "Menlo" || got.UI.Theme != "dark" {
		t.Errorf("ui = %+v", got.UI)
	}
	if len(issues) != 1 || issues[0].Field != "ui.textFont" || issues[0].Unreadable {
		t.Errorf("issues = %+v", issues)
	}
}

// GUI-025: the fonts' weights are CSS's 100 to 900 in steps of 100, or zero for
// the app's own; one saved comes back, a bad one is refused by Save and reported
// and ignored from the file.
func TestFontWeights(t *testing.T) {
	for _, w := range []int{100, 300, 400, 600, 900} {
		if !ValidWeight(w) {
			t.Errorf("%d refused", w)
		}
	}
	for _, w := range []int{0, -100, 50, 350, 1000, 401} {
		if ValidWeight(w) {
			t.Errorf("%d passed", w)
		}
	}
	dir := t.TempDir()
	c := Default()
	c.UI.TextWeight, c.UI.CodeWeight = 500, 300
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, issues, err := Load(dir)
	if err != nil || len(issues) != 0 || got.UI.TextWeight != 500 || got.UI.CodeWeight != 300 {
		t.Fatalf("Load = %+v, %v, %v", got.UI, issues, err)
	}
	c.UI.TextWeight = 450
	if err := Save(dir, c); err == nil {
		t.Error("a weight of 450 was saved")
	}
	got, issues, _ = Load(write(t, "version: 1\nui:\n  theme: dark\n  textWeight: 1200\n  codeWeight: 500\n"))
	if got.UI.TextWeight != 0 || got.UI.CodeWeight != 500 || len(issues) != 1 || issues[0].Field != "ui.textWeight" {
		t.Errorf("ui = %+v, issues = %+v", got.UI, issues)
	}
	// The app's own weight is left out of the file.
	if b, _ := yaml.Marshal(Default()); strings.Contains(string(b), "Weight") {
		t.Errorf("the default config writes a weight:\n%s", b)
	}
}
