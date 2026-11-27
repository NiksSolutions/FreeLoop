package engine

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// RFC 7386's own examples, and a number that must keep its digits.
func TestMergePatch(t *testing.T) {
	for _, c := range []struct{ doc, patch, want string }{
		{`{"a":"b"}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"b"}`, `{"b":"c"}`, `{"a":"b","b":"c"}`},
		{`{"a":"b"}`, `{"a":null}`, `{}`},
		{`{"a":"b","b":"c"}`, `{"a":null}`, `{"b":"c"}`},
		{`{"a":["b"]}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"c"}`, `{"a":["b"]}`, `{"a":["b"]}`},
		{`{"a":{"b":"c"}}`, `{"a":{"b":"d","c":null}}`, `{"a":{"b":"d"}}`},
		{`{"a":[{"b":"c"}]}`, `{"a":[1]}`, `{"a":[1]}`},
		{`{"e":null}`, `{"a":1}`, `{"a":1,"e":null}`},
		{`{"n":9007199254740993}`, `{"m":1}`, `{"m":1,"n":9007199254740993}`},
	} {
		got, err := mergePatch([]byte(c.doc), []byte(c.patch))
		if err != nil || string(got) != c.want {
			t.Errorf("%s + %s = %s, %v; want %s", c.doc, c.patch, got, err, c.want)
		}
	}
	if _, err := mergePatch([]byte(`{}`), []byte(`["a"]`)); err == nil {
		t.Error("a list was taken as a patch")
	}
}

// Issue 091: a window sends what it changed, and what others changed meanwhile stays.
func TestChangeSettingsKeepsWhatItDidNotChange(t *testing.T) {
	e, err := Open(Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	c := e.Config()
	c.UI.FontSize, c.UI.Language = 16, "de"
	if _, err := e.SaveSettings(c); err != nil {
		t.Fatal(err)
	}
	before := e.Config().Revision
	got, err := e.ChangeSettings(`{"ui":{"shortcuts":{"textLarger":"Meta+Shift+L"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.UI.Shortcuts["textLarger"] != "Meta+Shift+L" || got.UI.FontSize != 16 || got.UI.Language != "de" {
		t.Errorf("after the change: %+v", got.UI)
	}
	// Each config the engine holds is counted, so the window can drop an older answer.
	if got.Revision <= before || e.Config().Revision != got.Revision {
		t.Errorf("revision %d after %d, the engine's %d", got.Revision, before, e.Config().Revision)
	}
	if _, err := e.ChangeSettings(`{"revision":1}`); err != nil || e.Config().Revision != got.Revision+1 {
		t.Errorf("a revision sent was kept: %d, %v", e.Config().Revision, err)
	}
	got, err = e.ChangeSettings(`{"ui":{"shortcuts":{"textLarger":null}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.UI.Shortcuts["textLarger"]; ok || got.UI.FontSize != 16 {
		t.Errorf("after the Reset: %+v", got.UI)
	}
	if e.Config().UI.Language != "de" {
		t.Errorf("the engine's config: %+v", e.Config().UI)
	}
	for _, bad := range []string{`{"ui":{"fontSizee":12}}`, `["ui"]`, `{"ui":{"fontSize":"large"}}`} {
		if _, err := e.ChangeSettings(bad); err == nil {
			t.Errorf("%s was taken", bad)
		}
	}
	if e.Config().UI.FontSize != 16 {
		t.Errorf("a refused change changed the font size: %d", e.Config().UI.FontSize)
	}
}

// Changes from a window and the engine's own, made at once, are all kept.
func TestSettingsChangesAtOnceAreAllKept(t *testing.T) {
	e, err := Open(Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), WatchInterval: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := e.ChangeSettings(fmt.Sprintf(`{"ui":{"shortcuts":{"s%d":"Meta+%d"}}}`, i, i)); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := e.SetSkillEnabled(context.Background(), fmt.Sprintf("skill-%d", i), "", false); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	c := e.Config()
	for i := range n {
		if c.UI.Shortcuts[fmt.Sprintf("s%d", i)] == "" {
			t.Errorf("shortcut s%d was dropped", i)
		}
		if !slices.Contains(c.Skills.Disabled, fmt.Sprintf("skill-%d", i)) {
			t.Errorf("skill-%d was dropped", i)
		}
	}
}
