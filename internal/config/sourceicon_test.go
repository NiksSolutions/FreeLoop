package config

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

func pngURL(t *testing.T, w, h int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// GUI-034: a mark's picture is a small PNG as a data: URL, and nothing else; a bad one
// in the file is reported and dropped, the rest of the mark kept.
func TestSourceIcons(t *testing.T) {
	for _, v := range []string{pngURL(t, 64, 64), pngURL(t, 16, 9)} {
		if !ValidSourceIcon(v) {
			t.Errorf("refused %.40s", v)
		}
	}
	svg := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	for name, v := range map[string]string{
		"too large":      pngURL(t, 65, 10),
		"an SVG":         svg,
		"a URL":          "https://example.com/logo.png",
		"not base64":     "data:image/png;base64,!!!",
		"not a PNG":      "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("GIF89a")),
		"too many bytes": "data:image/png;base64," + strings.Repeat("A", 40000),
	} {
		if ValidSourceIcon(v) {
			t.Errorf("%s passed", name)
		}
	}
	got, issues := CleanSourceMarks(map[string]SourceMark{"a": {Slot: 2, Icon: svg}, "b": {Icon: pngURL(t, 32, 32)}})
	if got["a"].Icon != "" || got["a"].Slot != 2 || got["b"].Icon == "" || len(issues) != 1 || issues[0].Field != "ui.sourceMarks.a.icon" {
		t.Errorf("marks = %+v, issues = %+v", got, issues)
	}
}
