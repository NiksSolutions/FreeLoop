//go:build linux && !(cgo && gtk3)

package linux

// PrepareWebViews does nothing without cgo and the gtk3 tag: a test build, or
// Wails on GTK 4, whose web view the app does not ship (webview_gtk3.go).
func PrepareWebViews() {}
