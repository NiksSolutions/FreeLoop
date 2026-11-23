package windows

// PageOrigin is the origin of the app's own page in its window. Wails v3.0.0 serves the
// page to WebView2 from http://wails.localhost (internal/assetserver/
// assetserver_windows.go in its module; macOS and Linux get its wails: URL scheme), and
// Chromium sends that origin with every POST, the page's engine calls among them
// (netguard.RuntimeCallAllowed). Check it again when Wails moves: a wrong origin here
// refuses every call the window makes.
const PageOrigin = "http://wails.localhost"
