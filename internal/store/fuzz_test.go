package store

import (
	"bytes"
	"encoding/json"
	"testing"
)

// SES-002, NFR-REL-001: a transcript is read whatever a crash or a hand edit left in
// it: no line makes the reader panic, and what it returns are events after the seq
// asked for, at most the number asked for, each with its data as JSON.
func FuzzParseEvents(f *testing.F) {
	for _, seed := range []string{
		`{"v":1,"seq":1,"ts":"2026-11-20T10:00:00Z","sid":"s","type":"notice","data":{"message":"hi"}}` + "\n",
		`{"v":1,"seq":1,"type":"notice","data":{}}` + "\n" + `{"v":1,"seq":2,"type":"x"`,
		"\n\n{}\n[]\nnull\n",
		`{"seq":"1"}` + "\n" + `{"seq":-5,"data":"x"}` + "\n",
		"\xff\xfe\x00\n",
	} {
		f.Add([]byte(seed), int64(0), 0)
	}
	f.Fuzz(func(t *testing.T, data []byte, after int64, limit int) {
		if limit < 0 || limit > 1000 {
			return
		}
		events, err := parseEvents(bytes.NewReader(data), "events.jsonl", after, limit, 0)
		if err != nil {
			return
		}
		if limit > 0 && len(events) > limit {
			t.Fatalf("%d events for a limit of %d", len(events), limit)
		}
		for _, ev := range events {
			if ev.Seq <= after {
				t.Fatalf("seq %d after %d", ev.Seq, after)
			}
			if len(ev.Data) > 0 && !json.Valid(ev.Data) {
				t.Fatalf("data %q is not JSON", ev.Data)
			}
		}
	})
}
