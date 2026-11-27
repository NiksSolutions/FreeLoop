package engine

import (
	"bytes"
	"context"
	"html"
	"image/png"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// The terms behind which logos the app ships and which it does not (TRADEMARKS.md,
// frontend/src/logos/README.md, RISKS.md L-03), each by a phrase its page said when
// checked on 2026-11-27. A phrase gone means the terms changed: read the page again.
var logoTerms = []struct {
	who, url, phrase, choice string
}{
	{"llama.cpp", "https://raw.githubusercontent.com/ggml-org/llama-brand/master/BRAND-USAGE.md", "grants everyone permission to reproduce and redistribute these marks", "its logo ships"},
	{"OpenRouter", "https://openrouter.ai/brand", "stretch, recolor, or remix the marks", "its glyph ships"},
	{"vLLM", "https://raw.githubusercontent.com/vllm-project/media-kit/main/README.md", "Compact Logos", "its compact logo ships"},
	{"Anthropic", "https://www.anthropic.com/legal/trademark-guidelines", "only in materials we approve beforehand", "its logos do not ship"},
	{"OpenAI", "https://openai.com/brand/", "Use the logo without permission", "its logos do not ship"},
	{"OpenCode", "https://opencode.ai/brand", "Use a badge or a partner lockup", "its logo does not ship"},
}

var tags = regexp.MustCompile(`(?s)<[^>]*>`)

// GUI-034, before a release (make check-logos): each official logo still comes from the
// vendor's website as a picture, and the vendors' terms still say what the choice of
// logos rests on. It goes to the internet, so only when asked for, never in CI.
func TestLogosLive(t *testing.T) {
	if os.Getenv("FREELOOP_CHECK_LOGOS") == "" {
		t.Skip("goes to the vendors' websites: make check-logos")
	}
	client := &http.Client{Transport: http.DefaultTransport, CheckRedirect: httpsOnly}
	var vendors []string
	for v := range officialLogos {
		vendors = append(vendors, v)
	}
	slices.Sort(vendors)
	for _, v := range vendors {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		logo, err := fetchLogo(ctx, client, officialLogos[v])
		cancel()
		switch {
		case err != nil:
			t.Errorf("%s's official logo: %v; find its new address and change officialLogos", v, err)
			continue
		case len(logo.Data) < 200:
			t.Errorf("%s's official logo from %s is %d bytes, too few for a logo", v, logo.From, len(logo.Data))
			continue
		case logo.Type == "image/png":
			if cfg, err := png.DecodeConfig(bytes.NewReader(logo.Data)); err != nil || cfg.Width < 32 {
				t.Errorf("%s's official logo from %s: %v, %d pixels wide", v, logo.From, err, cfg.Width)
				continue
			}
		}
		t.Logf("logo  %-10s %-14s %8d bytes  %s", v, logo.Type, len(logo.Data), logo.From)
	}
	for _, term := range logoTerms {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		page, _, err := fetchCapped(ctx, client, term.url, maxLogoPage*4)
		cancel()
		if err != nil {
			t.Errorf("%s's terms (%s): %v", term.who, term.url, err)
			continue
		}
		text := strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(string(page), " "))), " ")
		text = strings.ReplaceAll(text, "’", "'")
		if !strings.Contains(text, term.phrase) {
			t.Errorf("%s's terms no longer say %q (%s): read them again, as %s on that ground, and update TRADEMARKS.md", term.who, term.phrase, term.url, term.choice)
			continue
		}
		t.Logf("terms %-10s still: %q", term.who, term.phrase)
	}
}
