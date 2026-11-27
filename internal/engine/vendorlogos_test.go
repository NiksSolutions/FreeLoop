package engine

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// GUI-034: the icons a page declares, largest first, https only, and not the one-colour
// mask icon a browser draws on a tab.
func TestDeclaredIcons(t *testing.T) {
	page := []byte(`<html><head>
<link rel="mask-icon" href="https://cdn.example/pin.svg" color="#000">
<link rel="icon" href="/small.png" sizes="32x32">
<link REL='apple-touch-icon-precomposed' href='https://cdn.example/big.png' sizes='512x512'>
<link rel="icon" href="http://cdn.example/plain.png" sizes="1024x1024">
<link rel="stylesheet" href="/site.css">
<link rel="shortcut icon" href="/favicon.ico">
<link rel="icon" href="https://cdn.example/big.png" sizes="512x512">
</head></html>`)
	got := declaredIcons("https://gemini.example/app", page)
	want := []string{"https://cdn.example/big.png", "https://gemini.example/small.png", "https://gemini.example/favicon.ico"}
	if !slices.Equal(got, want) {
		t.Errorf("icons = %q, want %q", got, want)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// GUI-034: a logo comes from the first address that serves a picture, over https, its
// type read from the bytes; a page's own icon is tried first; a redirect to http, an
// answer that is no picture and one too large are refused.
func TestFetchLogo(t *testing.T) {
	logo := pngBytes(t)
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(logo)
	})
	mux.HandleFunc("/logo.svg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`))
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<link rel="icon" href="/logo.svg" sizes="any">`))
	})
	mux.HandleFunc("/to-http", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host+"/logo.png", http.StatusFound)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("<html>not a logo</html>")) })
	client := srv.Client()
	client.CheckRedirect = httpsOnly
	ctx := context.Background()

	got, err := fetchLogo(ctx, client, logoSource{urls: []string{srv.URL + "/missing", srv.URL + "/logo.png"}})
	if err != nil || got.Type != "image/png" || !bytes.Equal(got.Data, logo) || got.From != srv.URL+"/logo.png" {
		t.Errorf("fallback: %v, %s from %s", err, got.Type, got.From)
	}
	if got, err := fetchLogo(ctx, client, logoSource{page: srv.URL + "/page", urls: []string{srv.URL + "/logo.png"}}); err != nil || got.Type != "image/svg+xml" || got.From != srv.URL+"/logo.svg" {
		t.Errorf("the page's icon: %v, %s from %s", err, got.Type, got.From)
	}
	for name, src := range map[string]logoSource{
		"a redirect to http": {urls: []string{srv.URL + "/to-http"}},
		"no picture":         {urls: []string{srv.URL + "/html"}},
		"an http address":    {urls: []string{strings.Replace(srv.URL, "https", "http", 1) + "/logo.png"}},
	} {
		if got, err := fetchLogo(ctx, client, src); err == nil {
			t.Errorf("%s: fetched %s", name, got.From)
		}
	}
	if _, _, err := fetchCapped(ctx, client, srv.URL+"/logo.png", int64(len(logo)-1)); err == nil {
		t.Error("an answer over the cap was taken")
	}
}

// The engine knows an address for every vendor the window has, less those whose logo
// it ships, and fetches nothing for another name or in local-only mode.
func TestOfficialLogosMatchTheWindow(t *testing.T) {
	read := func(p string) string {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	union := regexp.MustCompile(`export type Vendor = ([^;]+);`).FindStringSubmatch(read("../../frontend/src/vendors.ts"))
	if union == nil {
		t.Fatal("no Vendor type in vendors.ts")
	}
	shipped := regexp.MustCompile(`(?m)^  ([a-z]+): \{ onLight:`).FindAllStringSubmatch(read("../../frontend/src/VendorLogo.tsx"), -1)
	var want []string
	for _, m := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(union[1], -1) {
		if !slices.ContainsFunc(shipped, func(s []string) bool { return s[1] == m[1] }) {
			want = append(want, m[1])
		}
	}
	var got []string
	for k, src := range officialLogos {
		got = append(got, k)
		for _, u := range append(src.urls, src.page) {
			if u != "" && !strings.HasPrefix(u, "https://") {
				t.Errorf("%s: %s is not https", k, u)
			}
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if len(shipped) == 0 || !slices.Equal(got, want) {
		t.Errorf("official logos for %v, want %v (shipped %d)", got, want, len(shipped))
	}

	e := openTest(t, t.TempDir())
	defer e.Close()
	if _, err := e.FetchOfficialLogo(context.Background(), "acme"); err == nil {
		t.Error("fetched a logo for an unknown vendor")
	}
	was := netguard.LocalOnly()
	netguard.SetLocalOnly(true)
	defer netguard.SetLocalOnly(was)
	if _, err := e.FetchOfficialLogo(context.Background(), "claude"); err == nil || !strings.Contains(err.Error(), "nothing but this computer") {
		t.Errorf("local-only: %v", err)
	}
}
