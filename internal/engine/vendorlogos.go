package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Official logos (GUI-034): where each vendor's own website keeps its logo or app icon,
// for the marks of the vendors whose logo the app does not ship. The app fetches one only
// when the user clicks "Official logo" on a mark in Settings, and the window keeps it as
// the user's own picture, drawn into a small PNG (config.ValidSourceIcon). The addresses
// are fixed here, https and the vendors' own, so the window cannot make the engine fetch
// anything else. App icons with a background of their own come first, so the mark reads
// in both appearances. The keys are frontend/src/vendors.ts's Vendor, less those with a
// logo in frontend/src/logos.

type logoSource struct {
	urls []string // tried in order
	page string   // else first the icons this page declares, largest first
}

var officialLogos = map[string]logoSource{
	"anthropic": {urls: []string{"https://www.anthropic.com/favicon.ico"}},
	"claude":    {urls: []string{"https://claude.ai/apple-touch-icon.png"}},
	"openai":    {urls: []string{"https://openai.com/apple-icon.png"}},
	"gemini":    {page: "https://gemini.google.com/"},
	"ollama":    {urls: []string{"https://ollama.com/public/apple-touch-icon.png"}},
	"lmstudio":  {urls: []string{"https://lmstudio.ai/assets/marketing/brand/download/logos/lm-studio-icon-color.svg"}},
	"goose":     {urls: []string{"https://raw.githubusercontent.com/aaif-goose/goose/main/ui/desktop/src/images/icon.png"}},
	"opencode":  {urls: []string{"https://opencode.ai/brand/files/opencode-logo-dark-square.svg"}},
	// GitHub's own: Copilot has had no logo of its own since 2025 (brand.github.com).
	"copilot": {urls: []string{"https://github.githubassets.com/apple-touch-icon-180x180.png"}},
}

// OfficialLogo is a vendor's logo as its website serves it, for the window to draw.
type OfficialLogo struct {
	Data []byte `json:"data"`
	Type string `json:"type"`
	From string `json:"from"`
}

const (
	maxOfficialLogo = 4 << 20
	maxLogoPage     = 2 << 20
)

// FetchOfficialLogo fetches vendor's logo from its website, at the user's click.
func (e *Engine) FetchOfficialLogo(ctx context.Context, vendor string) (OfficialLogo, error) {
	src, ok := officialLogos[vendor]
	if !ok {
		return OfficialLogo{}, fmt.Errorf("no official logo is known for %q", vendor)
	}
	if netguard.LocalOnly() {
		return OfficialLogo{}, errors.New("the app is set to connect to nothing but this computer")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return fetchLogo(ctx, &http.Client{Transport: http.DefaultTransport, CheckRedirect: httpsOnly}, src)
}

// httpsOnly follows a few redirects, and only to https.
func httpsOnly(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("a redirect to %s, which is not https", req.URL.Redacted())
	}
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return nil
}

func fetchLogo(ctx context.Context, client *http.Client, src logoSource) (OfficialLogo, error) {
	urls := src.urls
	if src.page != "" {
		page, _, err := fetchCapped(ctx, client, src.page, maxLogoPage)
		if err != nil {
			return OfficialLogo{}, err
		}
		urls = append(declaredIcons(src.page, page), urls...)
	}
	err := errors.New("no address to fetch it from")
	for _, u := range urls {
		data, typ, ferr := fetchCapped(ctx, client, u, maxOfficialLogo)
		if ferr == nil && strings.HasPrefix(typ, "image/") {
			return OfficialLogo{Data: data, Type: typ, From: u}, nil
		}
		if ferr == nil {
			ferr = fmt.Errorf("%s is not a picture", u)
		}
		err = ferr
	}
	return OfficialLogo{}, err
}

// fetchCapped gets an https address, refusing an answer over max bytes, and says what it
// is: the type the server gave, or for a picture the type its bytes show.
func fetchCapped(ctx context.Context, client *http.Client, address string, max int64) ([]byte, string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" {
		return nil, "", fmt.Errorf("%q is not an https address", address)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Freeloop")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%s answered %s", u.Host, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > max {
		return nil, "", fmt.Errorf("%s sent more than %d bytes", u.Host, max)
	}
	return data, kindOf(res.Header.Get("Content-Type"), data), nil
}

// kindOf is a media type for data: an SVG by its markup, as servers and sniffing call it
// XML; a picture by its bytes; else what the server said.
func kindOf(header string, data []byte) string {
	head := bytes.ToLower(data[:min(len(data), 1024)])
	if bytes.Contains(head, []byte("<svg")) {
		return "image/svg+xml"
	}
	if sniffed := http.DetectContentType(data); strings.HasPrefix(sniffed, "image/") {
		return sniffed
	}
	typ, _, _ := strings.Cut(header, ";")
	return strings.TrimSpace(strings.ToLower(typ))
}

var (
	linkTag  = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	linkAttr = regexp.MustCompile(`(?is)\b(rel|href|sizes)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// declaredIcons are the https addresses of the icons a page declares in its link tags,
// the largest first; a mask icon, drawn in one colour for a browser's tab, is left out.
func declaredIcons(pageURL string, page []byte) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	type icon struct {
		address string
		size    int
	}
	var icons []icon
	for _, tag := range linkTag.FindAll(page, -1) {
		attrs := map[string]string{}
		for _, m := range linkAttr.FindAllSubmatch(tag, -1) {
			attrs[strings.ToLower(string(m[1]))] = string(m[2]) + string(m[3])
		}
		rel := strings.Fields(strings.ToLower(attrs["rel"]))
		if !slices.ContainsFunc(rel, func(r string) bool { return strings.Contains(r, "icon") && r != "mask-icon" }) {
			continue
		}
		u, err := base.Parse(strings.TrimSpace(attrs["href"]))
		if err != nil || u.Scheme != "https" {
			continue
		}
		size := 0
		for _, s := range strings.Fields(strings.ToLower(attrs["sizes"])) {
			if s == "any" {
				size = max(size, 1<<20)
			} else if w, _, ok := strings.Cut(s, "x"); ok {
				n, _ := strconv.Atoi(w)
				size = max(size, n)
			}
		}
		icons = append(icons, icon{u.String(), size})
	}
	slices.SortStableFunc(icons, func(a, b icon) int { return b.size - a.size })
	out := []string{}
	for _, i := range icons {
		if !slices.Contains(out, i.address) {
			out = append(out, i.address)
		}
	}
	return out
}
