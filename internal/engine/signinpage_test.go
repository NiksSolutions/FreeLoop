package engine

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// MCP-006: the browser gets the whole "Signed in" page, though the sign-in stops the
// loopback server the moment the code arrives. Closing it outright cut the page off
// (CI saw EOF on the redirect back).
func TestSignInPageReachesTheBrowser(t *testing.T) {
	for i := range 300 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		got := make(chan url.Values, 1)
		srv := &http.Server{Handler: callbackHandler("s", got)}
		go srv.Serve(ln)
		done := make(chan error, 1)
		go func() {
			resp, err := http.Get("http://" + ln.Addr().String() + "/callback?state=s&code=c")
			if err != nil {
				done <- err
				return
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err == nil && !strings.Contains(string(b), "Signed in") {
				err = fmt.Errorf("the page is %q", b)
			}
			done <- err
		}()
		<-got
		stopCallback(srv)
		if err := <-done; err != nil {
			t.Fatalf("try %d: %v", i, err)
		}
	}
}
