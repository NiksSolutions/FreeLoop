package main

import (
	"runtime"
	"testing"
)

// NFR-XPLAT-003: the engine calls of the window's page are taken from the origin the
// platform names: WebView2's on Windows, none where the page comes from the wails:
// scheme, which needs none.
func TestPageOriginIsThePlatforms(t *testing.T) {
	want := ""
	if runtime.GOOS == "windows" {
		want = "http://wails.localhost"
	}
	if thisPlatform.PageOrigin != want {
		t.Errorf("PageOrigin = %q, want %q", thisPlatform.PageOrigin, want)
	}
}
