package remote

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/event"
)

// pngPicture is a small real PNG, base64 encoded, as the phone app sends one.
func pngPicture(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// Issue 058: a message from the phone may carry pictures, or be pictures alone. The
// engine checks and stores them as on the computer, the message says it came from the
// phone, and the phone reads its pictures back with its key.
func TestPhoneSendsPictures(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	ctx := context.Background()
	ch, cancel := e.Subscribe()
	defer cancel()
	ses, err := e.CreateSession(ctx, engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	pic := pngPicture(t)
	prompt := "/api/sessions/" + ses.ID + "/prompt"
	if resp, out := p.do("POST", prompt, map[string]any{"text": "", "images": []map[string]string{{"mediaType": "image/png", "data": pic}}}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("a message of a picture alone: %d %v", resp.StatusCode, out)
	}
	text := waitEvent(t, ch, func(ev event.Event) bool { return ev.SID == ses.ID && ev.Type == event.UserMessageChunk })
	if !strings.Contains(string(text.Data), `"via":"phone:Test phone"`) {
		t.Errorf("the message does not say it came from the phone: %s", text.Data)
	}
	ev := waitEvent(t, ch, func(ev event.Event) bool {
		return ev.SID == ses.ID && ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"type":"image"`)
	})
	var chunk struct {
		Content struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
		} `json:"content"`
	}
	json.Unmarshal(ev.Data, &chunk)
	name, ok := strings.CutPrefix(chunk.Content.URI, "attachment:")
	if !ok || chunk.Content.MimeType != "image/png" {
		t.Fatalf("the picture is not one of the session's attachments: %s", ev.Data)
	}

	// The phone reads it back with its key, and only with it.
	path := "/api/sessions/" + ses.ID + "/attachments/" + name
	resp, out := p.do("GET", path, nil, nil)
	if resp.StatusCode != http.StatusOK || out["mediaType"] != "image/png" || out["data"] != pic {
		t.Fatalf("reading the picture back: %d %v", resp.StatusCode, out["mediaType"])
	}
	stranger := &phone{t: t, base: p.base, client: p.client}
	if resp, _ := stranger.do("GET", path, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("an unpaired caller read a picture: %d", resp.StatusCode)
	}
	for _, bad := range []string{"..%2Fdevices.json", "nothing-here.png"} {
		if resp, _ := p.do("GET", "/api/sessions/"+ses.ID+"/attachments/"+bad, nil, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("attachment %q: %d", bad, resp.StatusCode)
		}
	}

	// What the bytes are decides, not what the phone says; ten pictures at most; and an
	// empty message is still refused.
	refused := map[string]map[string]any{
		"not a PNG":    {"text": "x", "images": []map[string]string{{"mediaType": "image/png", "data": base64.StdEncoding.EncodeToString([]byte("GIF89a, not really"))}}},
		"eleven":       {"text": "x", "images": eleven(pic)},
		"empty":        {"text": " ", "images": []map[string]string{}},
		"long message": {"text": strings.Repeat("x", 65<<10)},
	}
	for what, body := range refused {
		if resp, _ := p.do("POST", prompt, body, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d", what, resp.StatusCode)
		}
	}
}

func eleven(pic string) []map[string]string {
	out := make([]map[string]string, engine.MaxImages+1)
	for i := range out {
		out[i] = map[string]string{"mediaType": "image/png", "data": pic}
	}
	return out
}

// Issue 058: the phone server takes a message with pictures up to what ten pictures at
// the store's limit need, and every other call stays within 64 KB.
func TestPhoneBodyLimits(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	ses, err := e.CreateSession(context.Background(), engine.CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "demo", Model: "scripted"})
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("A", 70<<10)
	if resp, _ := p.do("POST", "/api/decide", map[string]string{"requestId": big, "optionId": "x"}, nil); resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a 70 KB answer: %d", resp.StatusCode)
	}
	// A picture message past the limit is refused before it is read.
	huge := strings.Repeat("A", int(maxPictureBody)+1)
	if resp, _ := p.do("POST", "/api/sessions/"+ses.ID+"/prompt", map[string]any{"text": "x", "images": []map[string]string{{"mediaType": "image/png", "data": huge}}}, nil); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a message past the picture limit: %d", resp.StatusCode)
	}
}
