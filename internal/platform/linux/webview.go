package linux

import "net/url"

// Voice input in the Linux build (GUI-050, OQ-048). WebKitGTK gives a page no
// getUserMedia unless its web view's enable-media-stream setting is on, and Wails
// leaves it off; webview_gtk3.go turns it on in each web view as Wails puts the
// view in its window, before the page loads, and answers the view's requests for
// the microphone with mayRecord.
//
// This file has no build constraint, unlike most of the package, so that its
// tests run on every system.

// appScheme and appHost are where Wails serves the app's own page on Linux:
// wails://localhost/, with a port under `make dev`.
const appScheme, appHost = "wails", "localhost"

// mayRecord says whether a web view whose page is at page may capture: sound
// only, never the camera or the screen, and only for the app's own page. page
// is the address the view last committed to, not one it is still loading, so a
// page on its way out cannot ask in the name of the app's. WebKit does not say
// which frame asked; its permissions policy keeps the microphone from a frame
// of another origin unless the frame is given it with allow="microphone", which
// the app never does, so the preview of a local server (GUI-020) gets nothing.
func mayRecord(page string, audio, video, display bool) bool {
	if !audio || video || display {
		return false
	}
	u, err := url.Parse(page)
	return err == nil && u.Scheme == appScheme && u.Hostname() == appHost
}
