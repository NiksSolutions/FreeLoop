package netguard

import (
	"net/netip"
	"testing"
)

func TestRuntimeCallAllowed(t *testing.T) {
	for _, c := range []struct {
		method, origin, site, host string
		served, want               bool
	}{
		{"POST", "wails://wails.localhost", "", "wails.localhost", false, true},
		{"POST", "wails://wails", "", "", false, true},
		{"POST", "", "", "wails.localhost", false, true},
		{"POST", "http://localhost:34117", "", "localhost:34117", true, true},
		{"POST", "http://localhost:41000", "", "localhost:34117", true, false},
		{"POST", "wails://localhost", "", "localhost:34117", true, false},
		// A page picks the host of what it sends to the app's URL scheme.
		{"POST", "http://localhost:41000", "", "localhost:41000", false, false},
		{"POST", "http://localhost", "", "localhost", false, false},
		{"POST", "http://localhost:41000", "", "wails.localhost", false, false},
		{"POST", "https://evil.example", "", "wails.localhost", false, false},
		{"POST", "null", "", "wails.localhost", false, false},
		{"POST", "", "cross-site", "wails.localhost", false, false},
		{"POST", "", "same-site", "wails.localhost", false, false},
		{"POST", "", "same-origin", "wails.localhost", false, true},
		{"GET", "", "", "wails.localhost", false, false},
		{"GET", "wails://wails.localhost", "", "wails.localhost", false, false},
		{"OPTIONS", "http://localhost:41000", "", "wails.localhost", false, false},
	} {
		if got := RuntimeCallAllowed(c.method, c.origin, c.site, c.host, "client-1", "", c.served); got != c.want {
			t.Errorf("%s from %q to %q (served %v) = %v", c.method, c.origin, c.host, c.served, got)
		}
	}
	// As WebKit delivered a framed page's no-cors fetch to the packaged
	// app: no origin, and a page cannot add the client header to one.
	if RuntimeCallAllowed("POST", "", "", "localhost", "", "", false) {
		t.Error("a call without the client header was allowed")
	}
	if !RuntimeCallAllowed("POST", "", "", "localhost", "client-1", "", false) {
		t.Error("the app's own call was refused")
	}
}

// NFR-XPLAT-003: on Windows WebView2 serves the app's page from http://wails.localhost
// and Chromium puts that origin on every POST, so the platform names it and exactly it
// is taken, from the page itself; elsewhere no http origin is.
func TestRuntimeCallFromTheWindowsPage(t *testing.T) {
	const page = "http://wails.localhost"
	for _, c := range []struct {
		origin, site, page string
		want               bool
	}{
		{"http://wails.localhost", "", page, true},
		{"http://wails.localhost", "same-origin", page, true},
		{"http://wails.localhost", "same-site", page, false},
		{"http://wails.localhost", "cross-site", page, false},
		{"http://wails.localhost", "none", page, false},
		{"http://wails.localhost.evil.example", "", page, false},
		{"http://wails.localhost:1", "", page, false},
		{"https://wails.localhost", "", page, false},
		{"http://localhost:41000", "", page, false},
		{"http://wails.localhost", "", "", false},
	} {
		if got := RuntimeCallAllowed("POST", c.origin, c.site, "wails.localhost", "client-1", c.page, false); got != c.want {
			t.Errorf("from %q (Sec-Fetch-Site %q, the page at %q) = %v", c.origin, c.site, c.page, got)
		}
	}
	if RuntimeCallAllowed("GET", page, "", "wails.localhost", "client-1", page, false) || RuntimeCallAllowed("POST", page, "", "wails.localhost", "", page, false) {
		t.Error("a GET, or a call without the client header, was allowed from the Windows page")
	}
}

func TestPrivateAddrCoversEmbeddedAndSpecialRanges(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:127.0.0.1", "64:ff9b::a9fe:a9fe", "64:ff9b::7f00:1", "2002:7f00:1::", "::127.0.0.1", "192.0.0.1", "198.18.0.1", "fec0::1", "0.1.2.3", "100.64.0.1", "::"} {
		if !PrivateAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s is treated as public", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111", "64:ff9b::808:808", "2002:808:808::"} {
		if PrivateAddr(netip.MustParseAddr(s)) {
			t.Errorf("%s is treated as private", s)
		}
	}
}
