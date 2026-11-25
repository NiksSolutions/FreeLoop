package linux

import "testing"

// GUI-050, OQ-048: the microphone is the app's own page's, and only the microphone.
func TestMayRecord(t *testing.T) {
	for _, c := range []struct {
		page                  string
		audio, video, display bool
		want                  bool
	}{
		{"wails://localhost/", true, false, false, true},
		{"wails://localhost/?session=s-1", true, false, false, true},
		{"wails://localhost:5173/", true, false, false, true}, // make dev
		// Never the camera or the screen, not even with the microphone.
		{"wails://localhost/", false, true, false, false},
		{"wails://localhost/", true, true, false, false},
		{"wails://localhost/", true, false, true, false},
		{"wails://localhost/", false, false, false, false},
		// Nothing committed yet.
		{"", true, false, false, false},
		{"about:blank", true, false, false, false},
		// A page the window was led to, a local server's among them.
		{"http://localhost:3000/", true, false, false, false},
		{"http://127.0.0.1:5173/", true, false, false, false},
		{"https://example.com/", true, false, false, false},
		{"http://wails.localhost/", true, false, false, false}, // the Windows build's page
		{"file:///home/user/page.html", true, false, false, false},
		{"wails://localhost.example.com/", true, false, false, false},
		{"wails://example.com/", true, false, false, false},
		{"wails:localhost", true, false, false, false},
	} {
		if got := mayRecord(c.page, c.audio, c.video, c.display); got != c.want {
			t.Errorf("mayRecord(%q, audio %v, video %v, display %v) = %v", c.page, c.audio, c.video, c.display, got)
		}
	}
}
