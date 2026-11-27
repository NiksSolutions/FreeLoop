package event

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func schemaValidator(t *testing.T) *Validator {
	t.Helper()
	b, err := os.ReadFile("../../schemas/events.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewValidator(b)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// EXT-002: the validator passes what the app writes and catches what breaks the schema,
// so the store's check in tests (FREELOOP_CHECK_EVENTS) means something.
func TestValidatorCatchesWhatTheSchemaRefuses(t *testing.T) {
	v := schemaValidator(t)
	ev := func(typ Type, data string) Event {
		return Event{V: Version, Seq: 1, SID: "ses_1", TS: time.Now().UTC(), Type: typ, Data: json.RawMessage(data)}
	}
	for _, good := range []Event{
		ev(SessionStatus, `{"status":"running"}`),
		ev(AgentMessageChunk, `{"content":{"type":"text","text":"hi"}}`),
		ev(UsageUpdate, `{"inputTokens":3,"outputTokens":1,"cacheReadTokens":0,"cacheWriteTokens":0,"measured":"turn"}`),
		ev(TurnEnded, `{"stopReason":"end_turn"}`),
	} {
		if err := v.Validate(good); err != nil {
			t.Errorf("a good event was refused: %v", err)
		}
	}
	for want, bad := range map[string]Event{
		"not one of":  ev(SessionStatus, `{"status":"sleeping"}`),
		"not of type": ev(UsageUpdate, `{"inputTokens":"three"}`),
		"type":        ev(Type("made_up"), `{}`),
		"measured":    ev(UsageUpdate, `{"measured":"guessed"}`),
	} {
		err := v.Validate(bad)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

// A built-in session's context: a message at an index, or, after a rewind to before
// the first message, cleared, which has no message (found by
// TestRewindWaitsForATurnThatIsEnding, 2026-11-27).
func TestHistoryMessages(t *testing.T) {
	v := schemaValidator(t)
	ev := func(data string) Event {
		return Event{V: Version, Seq: 1, SID: "ses_1", TS: time.Now().UTC(), Type: HistoryMessage, Data: json.RawMessage(data)}
	}
	msg := `{"role":"user","parts":[{"type":"text","text":"hi"}]}`
	for _, good := range []string{
		`{"index":0,"message":` + msg + `}`,
		`{"index":0,"reset":true,"message":` + msg + `}`,
		`{"index":0,"reset":true,"clear":true}`,
	} {
		if err := v.Validate(ev(good)); err != nil {
			t.Errorf("%s was refused: %v", good, err)
		}
	}
	for _, bad := range []string{
		`{"index":2}`,
		`{"index":1,"clear":false}`,
		`{"index":1,"reset":true,"clear":true}`,
		`{"index":0,"clear":true}`,
	} {
		if err := v.Validate(ev(bad)); err == nil {
			t.Errorf("%s was taken", bad)
		}
	}
}
