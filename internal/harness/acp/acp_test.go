package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/harness/acp/acptest"
	"github.com/FinanceNik/Freeloop/internal/permission"
)

func TestMain(m *testing.M) {
	if acptest.Serve() {
		return
	}
	os.Exit(m.Run())
}

type recorder struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recorder) emit(typ event.Type, data any) {
	b, _ := json.Marshal(data)
	r.mu.Lock()
	r.events = append(r.events, event.Event{Type: typ, Data: b})
	r.mu.Unlock()
}

func (r *recorder) text(typ event.Type) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sb strings.Builder
	for _, e := range r.events {
		if e.Type == typ {
			sb.Write(e.Data)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// HRN-005, MCP-004: a reference agent edits a file with the edit approved
// through the broker, receives the session's MCP servers, and a terminal
// request it did not ask permission for goes to the broker too.
func TestReferenceAgent(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	s, err := h.NewSession(context.Background(), harness.Params{
		SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit,
		MCPServers: []harness.MCPServerSpec{
			{Name: "github", Transport: "stdio", Command: "gh-mcp", Env: map[string]string{"TOKEN": "x"}},
			{Name: "remote", Transport: "http", URL: "https://example.com/mcp"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	stop := make(chan struct{})
	defer close(stop)
	var mu sync.Mutex
	var asked []permission.Request
	go func() {
		// A request still listed on the next look was already answered: once each
		// (a dry run of d9ababe counted one twice).
		seen := map[string]bool{}
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
			}
			for _, r := range broker.Pending() {
				if seen[r.ID] {
					continue
				}
				seen[r.ID] = true
				mu.Lock()
				asked = append(asked, r)
				mu.Unlock()
				opt := r.Options[0].ID
				if r.Tool == "shell" {
					opt = "allow_once"
				}
				broker.Decide(r.ID, opt, "", "")
			}
		}
	}()
	reason, err := s.Prompt(context.Background(), harness.Prompt{Text: "edit and use the terminal"})
	if err != nil || reason != harness.StopEndTurn {
		t.Fatalf("Prompt = %s, %v", reason, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(b) != "hello edited" {
		t.Errorf("notes.txt = %q", b)
	}
	mu.Lock()
	if len(asked) != 2 || asked[0].Tool != "edit_file" || asked[0].Paths[0] != filepath.Join(env.Root(), "notes.txt") || asked[1].Tool != "shell" || asked[1].Command != "sh -c 'echo from-terminal'" {
		t.Errorf("asked = %+v", asked)
	}
	mu.Unlock()
	msgs := rec.text(event.AgentMessageChunk)
	if !strings.Contains(msgs, "mcp: github=gh-mcp,remote=https://example.com/mcp") || !strings.Contains(msgs, "terminal: from-terminal") {
		t.Errorf("agent said %s", msgs)
	}
	if calls := rec.text(event.ToolCall); !strings.Contains(calls, `"tool":"edit_file"`) || strings.Contains(calls, "sessionUpdate") {
		t.Errorf("tool_call events = %s", calls)
	}
	if !strings.Contains(rec.text(event.UsageUpdate), `"inputTokens":10`) || !strings.Contains(rec.text(event.TurnEnded), "end_turn") {
		t.Error("usage or turn end missing")
	}
}

// PRV-008, GUI-023: an agent that reports each model call has each recorded once,
// with the stream's final output, its model and its hour-long cache writes. A result
// adds what the messages did not show, and a call cut off by the end of the turn
// still counts. The turn's own usage would count it twice, so it is left
// out.
func TestUsagePerModelCall(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}, Skills: config.SkillsClaudePlugin}}
	s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeBypass, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Prompt(context.Background(), harness.Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	var got []event.UsageData
	rec.mu.Lock()
	for _, e := range rec.events {
		if e.Type == event.UsageUpdate {
			var d event.UsageData
			json.Unmarshal(e.Data, &d)
			got = append(got, d)
		}
	}
	rec.mu.Unlock()
	want := []event.UsageData{
		{InputTokens: 3, OutputTokens: 4, CacheWriteTokens: 20, CacheWrite1hTokens: 20, Model: "test-model"},
		{InputTokens: 2, OutputTokens: 192, CacheReadTokens: 100, CacheWriteTokens: 50, CacheWrite1hTokens: 50, ContextTokens: 152, Model: "test-model"},
		{InputTokens: 1000, OutputTokens: 80, CacheWriteTokens: 30, Model: "test-model", Adjustment: true},
		{ContextTokens: 150, ContextWindow: 1000},
		{InputTokens: 5, OutputTokens: 1, CacheReadTokens: 200, ContextTokens: 205, ContextWindow: 1000, Model: "test-model"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("usage events = %+v, want %+v", got, want)
	}
}

// PRV-008: an agent that reports only a turn's total has it priced by the model it
// says it is on, and a turn it reports nothing for is said to have none, never $0.
func TestUsagePerTurn(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello"), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	for _, c := range []struct {
		name, prompt string
		model        bool
		stated       string
		want         event.UsageData
	}{
		{"no model setting", "go", false, "", event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn}},
		{"a model setting", "go", true, "", event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn, Model: "agent-model"}},
		// A profile names the model an agent that says none runs on, for its price;
		// what an agent says wins.
		{"the profile's model", "go", false, "gemini-2.5-pro", event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn, Model: "gemini-2.5-pro"}},
		{"the agent's model over the profile's", "go", true, "gemini-2.5-pro", event.UsageData{InputTokens: 10, OutputTokens: 5, Measured: event.MeasuredTurn, Model: "agent-model"}},
		{"nothing reported", "silent", true, "", event.UsageData{Measured: event.MeasuredNone}},
	} {
		rec := &recorder{}
		broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
		vars := map[string]string{acptest.EnvVar: "1"}
		if c.model {
			vars[acptest.ModelVar] = "1"
		}
		h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: vars}}
		s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeBypass, Emit: rec.emit, Model: c.stated})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Prompt(context.Background(), harness.Prompt{Text: c.prompt}); err != nil {
			t.Fatal(err)
		}
		s.Close()
		var got []event.UsageData
		rec.mu.Lock()
		for _, e := range rec.events {
			if e.Type == event.UsageUpdate {
				var d event.UsageData
				json.Unmarshal(e.Data, &d)
				got = append(got, d)
			}
		}
		rec.mu.Unlock()
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: usage events = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A write the agent did not ask permission for is refused when the user
// denies it, and a protected path is refused by the environment anyway.
func TestUnaskedWritesGoThroughTheBroker(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: rec.emit}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	go func() {
		for range 200 {
			for _, r := range broker.Pending() {
				broker.Decide(r.ID, "reject_once", "", "")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: filepath.Join(root, "x.txt"), Content: "x"}); err == nil {
		t.Error("an unasked write was allowed")
	}
	if _, err := os.Stat(filepath.Join(root, "x.txt")); err == nil {
		t.Error("the file was written")
	}
	if _, err := s.WriteTextFile(context.Background(), sdk.WriteTextFileRequest{Path: filepath.Join(root, ".git", "config"), Content: "x"}); err == nil {
		t.Error("a write into .git was allowed")
	}
}

// Review finding: an agent's current_mode_update was taken as it came, so it
// could put the session in bypass and approve itself. It can tighten the mode;
// it loosens it only after the user approved its request, and never to bypass.
func TestAgentCannotLoosenTheMode(t *testing.T) {
	rec := &recorder{}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModePlan, preset: config.HarnessPreset{Name: "Agent"}}
	// A refused change tells the agent again in the background, which reads these.
	set := func(m permission.Mode, a approval) {
		s.mu.Lock()
		s.mode, s.approved = m, a
		s.mu.Unlock()
	}
	for _, m := range []permission.Mode{permission.ModeBypass, permission.ModeAcceptEdits, permission.ModeAsk} {
		s.agentMode(m)
		if s.Mode() != permission.ModePlan {
			t.Fatalf("the agent changed plan mode to %s on its own", m)
		}
	}
	if strings.Contains(rec.text(event.CurrentModeUpdate), "bypass") {
		t.Error("the refused change reached the transcript")
	}
	if !strings.Contains(rec.text(event.ModeRefused), `"to":"bypass"`) {
		t.Error("the user was not told")
	}

	set(permission.ModePlan, approval{kind: permission.KindSwitchMode, until: time.Now().Add(time.Minute)})
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModePlan {
		t.Error("an approved switch reached bypass")
	}
	s.agentMode(permission.ModeAcceptEdits)
	if s.Mode() != permission.ModeAcceptEdits {
		t.Errorf("an approved switch left the mode %s", s.Mode())
	}
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeAcceptEdits {
		t.Error("the approval was used twice")
	}
	set(permission.ModePlan, approval{kind: permission.KindEdit, until: time.Now().Add(time.Minute)})
	s.agentMode(permission.ModeAsk)
	if s.Mode() != permission.ModePlan {
		t.Error("an approval of something else loosened the mode")
	}

	set(permission.ModeAcceptEdits, approval{})
	s.agentMode(permission.ModePlan)
	if s.Mode() != permission.ModePlan {
		t.Error("the agent could not tighten the mode")
	}
}

// Auto or bypass the user chose approving the agent's plan holds when the app allows
// it, and otherwise the session asks: it used to stay in plan mode, and said the
// agent had tried it on its own (2026-11-06).
func TestTheUserChoosesTheModeApprovingAPlan(t *testing.T) {
	rec := &recorder{}
	var refusal error
	modes := map[string]string{"ask": "default", "accept_edits": "default", "plan": "plan", "auto": "auto", "bypass": "default"}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit, AllowUnattended: func(permission.Mode) error { return refusal }}, preset: config.HarnessPreset{Name: "Agent", Modes: modes}}
	approve := func(byUser bool) {
		s.mu.Lock()
		s.mode = permission.ModePlan
		s.approved = approval{kind: permission.KindSwitchMode, until: time.Now().Add(time.Minute), byUser: byUser}
		s.mu.Unlock()
	}

	approve(true)
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeBypass {
		t.Errorf("the user's choice of bypass left %s", s.Mode())
	}

	approve(true)
	refusal = errors.New("bypass mode is off")
	s.agentMode(permission.ModeBypass)
	if s.Mode() != permission.ModeAsk {
		t.Errorf("a refused choice left %s, not ask", s.Mode())
	}
	if got := rec.text(event.ModeRefused); !strings.Contains(got, `"chosen":true`) || !strings.Contains(got, "bypass mode is off") {
		t.Errorf("the user was not told why: %s", got)
	}

	// A rule or the mode approving the plan is no choice of the user's.
	approve(false)
	refusal = nil
	s.agentMode(permission.ModeAuto)
	if s.Mode() != permission.ModePlan {
		t.Errorf("an approval not by the user reached %s", s.Mode())
	}

	// The user chose the option that asks before edits; an agent reporting its auto
	// mode after that has not been approved into it.
	approve(true)
	s.mu.Lock()
	s.approved.option = "default"
	s.mu.Unlock()
	s.agentReported("auto")
	if s.Mode() == permission.ModeAuto {
		t.Error("the agent chose auto over the user's choice")
	}
	approve(true)
	s.mu.Lock()
	s.approved.option = "auto"
	s.mu.Unlock()
	s.agentReported("auto")
	if s.Mode() != permission.ModeAuto {
		t.Errorf("the user's own choice of auto left %s", s.Mode())
	}
}

// HRN-007: several of the app's modes can share one of the agent's, and a mode
// the preset does not name changes nothing.
func TestAgentReportsItsMode(t *testing.T) {
	rec := &recorder{}
	preset := config.HarnessPreset{Name: "Agent", Modes: map[string]string{"ask": "asks", "accept_edits": "asks", "plan": "reads", "bypass": "asks"}}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModeAcceptEdits, preset: preset}
	set := func(m permission.Mode) {
		s.mu.Lock()
		s.mode = m
		s.mu.Unlock()
	}
	s.agentReported("asks")
	if s.Mode() != permission.ModeAcceptEdits {
		t.Errorf("reporting the mode it is in left %s", s.Mode())
	}
	set(permission.ModePlan)
	s.agentReported("asks")
	if s.Mode() != permission.ModePlan {
		t.Errorf("a loosening report took effect: %s", s.Mode())
	}
	set(permission.ModeAsk)
	s.agentReported("reads")
	if s.Mode() != permission.ModePlan {
		t.Errorf("tightening left %s", s.Mode())
	}
	// With no mapping for it there is no agent to call back, so this ends here.
	u := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, mode: permission.ModeAsk, preset: config.HarnessPreset{Name: "Agent", Modes: map[string]string{"ask": "asks"}}}
	u.agentReported("yolo")
	if u.Mode() != permission.ModeAsk || !strings.Contains(rec.text(event.Notice), "yolo") {
		t.Errorf("an unnamed mode: %s, %s", u.Mode(), rec.text(event.Notice))
	}
}

// HRN-007: a file that is not there is the protocol's "not found", which is
// what an agent about to create it checks for, and not a failure.
func TestReadingAMissingFile(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, _ := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root)})
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	s := &session{p: harness.Params{SessionID: "x", Broker: broker, Emit: rec.emit}, env: env, mode: permission.ModeAsk, terms: map[string]*terminal{}}
	_, err := s.ReadTextFile(context.Background(), sdk.ReadTextFileRequest{Path: filepath.Join(env.Root(), "new.txt")})
	var re *sdk.RequestError
	if !errors.As(err, &re) || re.Code != resourceNotFound {
		t.Errorf("reading a missing file: %v", err)
	}
	s.preset.MissingFileIsEmpty = true
	res, err := s.ReadTextFile(context.Background(), sdk.ReadTextFileRequest{Path: filepath.Join(env.Root(), "new.txt")})
	if err != nil || res.Content != "" {
		t.Errorf("with MissingFileIsEmpty: %q, %v", res.Content, err)
	}
}

// SEC-006: an agent that runs a project's hooks by itself, as Copilot CLI does over
// ACP, does not start in an untrusted project that has any: a hook can run anything,
// and approve the agent's tool calls before the broker is asked. Settings without
// hooks, or a trusted project, start as before.
func TestRepoHooksKeepAnAgentFromStarting(t *testing.T) {
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"},
		RepoHooks: []string{".github/hooks", ".claude/settings.json"}}}
	for _, c := range []struct {
		name    string
		files   map[string]string
		trusted bool
		refused string
	}{
		{name: "no hooks"},
		{name: "settings without hooks", files: map[string]string{".claude/settings.json": `{"hooks":{},"permissions":{"allow":["Bash"]}}`}},
		{name: "a hooks folder", files: map[string]string{".github/hooks/allow.json": `{"version":1}`, ".github/hooks/README.md": "x"}, refused: ".github/hooks/allow.json"},
		{name: "hooks in settings", files: map[string]string{".claude/settings.json": `{"hooks":{"PreToolUse":[]}}`}, refused: ".claude/settings.json"},
		{name: "settings it cannot read", files: map[string]string{".claude/settings.json": `{"hooks":`}, refused: ".claude/settings.json"},
		{name: "trusted", files: map[string]string{".github/hooks/allow.json": `{"version":1}`}, trusted: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, "repo")
			for name, text := range c.files {
				writeFile(t, filepath.Join(root, name), text)
			}
			os.MkdirAll(root, 0o755)
			policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
			env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
			if err != nil {
				t.Fatal(err)
			}
			rec := &recorder{}
			broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
			s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit, Trusted: func() bool { return c.trusted }})
			if err == nil {
				s.Close()
			}
			switch {
			case c.refused == "" && err != nil:
				t.Errorf("did not start: %v", err)
			case c.refused != "" && err == nil:
				t.Errorf("started with %s in an untrusted project", c.refused)
			case c.refused != "" && (!strings.Contains(err.Error(), c.refused) || !strings.Contains(err.Error(), "Trust the project")):
				t.Errorf("the refusal does not name %s or what to do: %v", c.refused, err)
			}
		})
	}
}

// HRN-007: a message an agent sends for machines stays out of the conversation.
func TestHiddenAgentText(t *testing.T) {
	rec := &recorder{}
	s := &session{p: harness.Params{SessionID: "x", Emit: rec.emit}, preset: config.HarnessPreset{HideText: []string{"[MODE_UPDATE]"}}}
	for _, text := range []string{"[MODE_UPDATE] plan", "real words"} {
		if err := s.SessionUpdate(context.Background(), sdk.SessionNotification{SessionId: "x", Update: sdk.UpdateAgentMessageText(text)}); err != nil {
			t.Fatal(err)
		}
	}
	got := rec.text(event.AgentMessageChunk)
	if strings.Contains(got, "MODE_UPDATE") || !strings.Contains(got, "real words") {
		t.Errorf("conversation = %s", got)
	}
}

// SEC-003: the line judged for an agent's terminal is what runs. A variable whose name
// is not one, which once made `git x=1 sh evil.sh` of `sh evil.sh` and so passed an
// allow rule for git, is refused; arguments and values are quoted as the shell reads
// them, so a rule sees each as one word.
func TestATerminalIsJudgedAsItRuns(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	env, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	hs, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	defer hs.Close()
	s := hs.(*session)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: "s1", Command: "sh", Args: []string{"evil.sh"}, Env: []sdk.EnvVariable{{Name: "git x", Value: "1"}}}); err == nil || !strings.Contains(err.Error(), "not a variable name") {
		t.Errorf("a variable named %q: %v", "git x", err)
	}
	// Asked about, the line names each word as it runs; refused here, nothing runs.
	go func() {
		for range 200 {
			if p := broker.Pending(); len(p) == 1 {
				if got := p[0].Command; got != `GIT_X='a; rm -rf ~' sh -c 'echo hi'` {
					t.Errorf("judged %q", got)
				}
				broker.Decide(p[0].ID, "reject_once", "", "")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("nothing was asked")
	}()
	if _, err := s.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: "s1", Command: "sh", Args: []string{"-c", "echo hi"}, Env: []sdk.EnvVariable{{Name: "GIT_X", Value: "a; rm -rf ~"}}}); err == nil {
		t.Error("a refused terminal ran")
	}
}

// execRecorder is an environment that keeps what it was asked to run.
type execRecorder struct {
	*environment.Local
	specs chan environment.ExecSpec
}

func (r execRecorder) Exec(ctx context.Context, spec environment.ExecSpec) (environment.ExecResult, error) {
	r.specs <- spec
	return environment.ExecResult{}, nil
}

// Security review, 2026-11-13: the variables an agent names for its terminal are the
// request's, shown in what was approved, and are passed as such, so an environment
// that runs commands through another program, as Docker does through its CLI on this
// computer, can keep them out of that program's own environment.
func TestATerminalsVariablesAreTheRequests(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	local, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	env := execRecorder{Local: local, specs: make(chan environment.ExecSpec, 1)}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	hs, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeAsk, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	defer hs.Close()
	s := hs.(*session)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		for range 400 {
			if p := broker.Pending(); len(p) == 1 {
				broker.Decide(p[0].ID, "allow_once", "", "")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	if _, err := s.CreateTerminal(ctx, sdk.CreateTerminalRequest{SessionId: "s1", Command: "true", Env: []sdk.EnvVariable{{Name: "DOCKER_CONFIG", Value: "cfg"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case spec := <-env.specs:
		if len(spec.Env) != 0 || spec.RequestEnv["DOCKER_CONFIG"] != "cfg" {
			t.Errorf("Env = %v, RequestEnv = %v", spec.Env, spec.RequestEnv)
		}
	case <-ctx.Done():
		t.Fatal("nothing ran")
	}
}

// refAgent starts the reference agent in bypass mode on a project whose notes.txt holds
// notes, in the environment wrap makes of the project's own.
func refAgent(t *testing.T, notes string, wrap func(*environment.Local) environment.Environment) (harness.Session, *recorder) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "repo")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte(notes), 0o644)
	policy := permission.NewPolicy(home, filepath.Join(home, "appdata"))
	local, err := environment.NewLocal(environment.LocalOptions{Root: root, Guard: policy.Guard(root), Shell: []string{"sh", "-c"}})
	if err != nil {
		t.Fatal(err)
	}
	var env environment.Environment = local
	if wrap != nil {
		env = wrap(local)
	}
	rec := &recorder{}
	broker := permission.NewBroker(policy, func(_ string, typ event.Type, data any) { rec.emit(typ, data) })
	exe, _ := os.Executable()
	h := Harness{Preset: config.HarnessPreset{ID: "ref", Name: "Reference", Command: exe, Env: map[string]string{acptest.EnvVar: "1"}}}
	s, err := h.NewSession(context.Background(), harness.Params{SessionID: "ses", Env: env, Broker: broker, Mode: permission.ModeBypass, Emit: rec.emit})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, rec
}

// said is what the agent said last.
func said(rec *recorder) string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for i := len(rec.events) - 1; i >= 0; i-- {
		if rec.events[i].Type == event.AgentMessageChunk {
			var d struct{ Content struct{ Text string } }
			json.Unmarshal(rec.events[i].Data, &d)
			return d.Content.Text
		}
	}
	return ""
}

// prompt sends text and fails the test unless the turn ends as turns do.
func prompt(t *testing.T, s harness.Session, text string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if reason, err := s.Prompt(ctx, harness.Prompt{Text: text}); err != nil || reason != harness.StopEndTurn {
		t.Fatalf("%s: %s, %v", text, reason, err)
	}
}

// HRN-005, NFR-REL-002: a read whose line and limit add up past the largest number
// returns the lines from its line on, which once overflowed and took the app down
// with every session in it; a limit of zero or less reads to the end, as before.
func TestAReadsLimitCannotOverflow(t *testing.T) {
	s, rec := refAgent(t, "a\nb\nc\nd", nil)
	for _, c := range []struct{ line, limit, want string }{
		{"2", "9223372036854775807", "b\nc\nd"},
		{"9223372036854775807", "9223372036854775807", ""},
		{"2", "0", "b\nc\nd"},
		{"2", "-1", "b\nc\nd"},
		{"2", "-9223372036854775808", "b\nc\nd"},
		{"2", "2", "b\nc"},
		{"-", "2", "a\nb"},
		{"-", "-", "a\nb\nc\nd"},
	} {
		prompt(t, s, "READ notes.txt "+c.line+" "+c.limit)
		if got := said(rec); got != "read: "+c.want {
			t.Errorf("line %s, limit %s: %q", c.line, c.limit, got)
		}
	}
}

// HRN-005: a terminal keeps at most 1 MB of output, whatever outputByteLimit the agent
// asks for; a smaller limit is kept.
func TestATerminalsOutputLimitIsCapped(t *testing.T) {
	s, rec := refAgent(t, "", nil)
	for _, c := range []struct{ limit, want string }{
		{"9223372036854775807", "output: 1048576 bytes, truncated true"},
		{"100", "output: 100 bytes, truncated true"},
	} {
		prompt(t, s, "RUN "+c.limit+" yes | head -c 3000000")
		if got := said(rec); got != c.want {
			t.Errorf("outputByteLimit %s: %q", c.limit, got)
		}
	}
}

// HRN-005: terminal/output returns what a command has printed so far while it
// runs, so an agent watching a dev server or a test watch sees it.
func TestATerminalsOutputShowsWhileItRuns(t *testing.T) {
	s, rec := refAgent(t, "", nil)
	prompt(t, s, "WATCH echo ready; sleep 0.5")
	if got := rec.text(event.AgentMessageChunk); !strings.Contains(got, "output: ready, ended false") || !strings.Contains(got, `"ended"`) {
		t.Errorf("the agent said %s", got)
	}
}

// HRN-005: past its limit a terminal keeps the latest output, cut at whole
// characters, as the protocol asks; a character still being written waits for the
// next read.
func TestATerminalsOutputKeepsTheEnd(t *testing.T) {
	o := &termOutput{limit: 5}
	o.Write([]byte("ab\xc3"))
	if got, truncated := o.text(); got != "ab" || truncated {
		t.Errorf("half a character: %q, %v", got, truncated)
	}
	o.Write([]byte("\xa9"))
	if got, truncated := o.text(); got != "abé" || truncated {
		t.Errorf("the whole character: %q, %v", got, truncated)
	}
	for range 10 {
		o.Write([]byte("wörld"))
	}
	if got, truncated := o.text(); got != "rld" && got != "örld" || !truncated || !utf8.ValidString(got) {
		t.Errorf("past the limit: %q, %v", got, truncated)
	}
	if len(o.buf) > 2*o.limit {
		t.Errorf("holds %d bytes", len(o.buf))
	}
}

// HRN-005: a turn that ends cancelled stops every terminal the agent started in it,
// so Stop does not leave an approved command running; one started in an earlier turn,
// which an agent may read in a later one, runs on.
func TestStopEndsTheTurnsTerminals(t *testing.T) {
	s, rec := refAgent(t, "", nil)
	root := s.(*session).env.Root()
	prompt(t, s, "BG sleep 30")
	if got := said(rec); got != "started term-1" {
		t.Fatalf("the agent said %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type ended struct {
		stop harness.StopReason
		err  error
	}
	done := make(chan ended, 1)
	go func() {
		stop, err := s.Prompt(ctx, harness.Prompt{Text: "WATCH echo ready; sleep 2; echo late > survived"})
		done <- ended{stop, err}
	}()
	for deadline := time.Now().Add(10 * time.Second); !strings.HasPrefix(said(rec), "output: ready"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the command did not start: %q", said(rec))
		}
	}
	stopped := time.Now()
	cancel()
	select {
	case e := <-done:
		if e.stop != harness.StopCancelled || e.err != nil {
			t.Errorf("turn ended %s, %v", e.stop, e.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the turn did not end")
	}
	if waited := time.Since(stopped); waited > time.Second {
		t.Errorf("the turn took %s to end after Stop", waited)
	}
	time.Sleep(time.Until(stopped.Add(2500 * time.Millisecond)))
	if _, err := os.Stat(filepath.Join(root, "survived")); err == nil {
		t.Error("the stopped turn's command ran on")
	}
	bg, err := s.(*session).terminal("term-1")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-bg.done:
		t.Error("Stop ended a terminal of an earlier turn")
	default:
	}
}

// faulty is a project environment with a bug: it panics resolving a path that ends in
// panic.txt and running a command that says PANIC.
type faulty struct{ *environment.Local }

func (f faulty) Resolve(path string) string {
	if strings.HasSuffix(path, "panic.txt") {
		panic("a bug in Resolve")
	}
	return f.Local.Resolve(path)
}

func (f faulty) Exec(ctx context.Context, spec environment.ExecSpec) (environment.ExecResult, error) {
	if strings.Contains(strings.Join(spec.Argv, " "), "PANIC") {
		panic("a bug in Exec")
	}
	return f.Local.Exec(ctx, spec)
}

// lockedBuffer is a log the handlers' goroutines can write while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// NFR-REL-002: a panic in a handler the client answers an agent with, which the
// protocol library runs in a goroutine with no recover, is an error to the agent and a
// line in the log with its stack; one in a terminal's command is logged too. The app
// and the session go on.
func TestAPanicInAHandlerIsAnError(t *testing.T) {
	var logged lockedBuffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	s, rec := refAgent(t, "hello", func(l *environment.Local) environment.Environment { return faulty{l} })
	prompt(t, s, "READ panic.txt")
	if got := said(rec); !strings.HasPrefix(got, "read failed: ") {
		t.Errorf("the agent was told %q", got)
	}
	if l := logged.String(); !strings.Contains(l, "fs/read_text_file") || !strings.Contains(l, "a bug in Resolve") || !strings.Contains(l, "acp_test.go") {
		t.Errorf("logged %q", l)
	}
	prompt(t, s, "RUN - echo PANIC")
	if got := said(rec); !strings.HasPrefix(got, "output: 0 bytes") {
		t.Errorf("the agent was told %q", got)
	}
	if l := logged.String(); !strings.Contains(l, "a bug in Exec") {
		t.Errorf("logged %q", l)
	}
	prompt(t, s, "READ notes.txt")
	if got := said(rec); got != "read: hello" {
		t.Errorf("after the panics the agent was told %q", got)
	}
}

// NFR-REL-002: every handler the session has for an agent's requests is guarded, since
// the protocol library finds them on the value it is given and offers no other.
func TestEveryHandlerIsGuarded(t *testing.T) {
	for _, iface := range []reflect.Type{reflect.TypeFor[sdk.Client](), reflect.TypeFor[sdk.ClientExperimental](), reflect.TypeFor[sdk.ExtensionMethodHandler]()} {
		for i := range iface.NumMethod() {
			name := iface.Method(i).Name
			if _, has := reflect.TypeFor[*session]().MethodByName(name); !has {
				continue
			}
			if _, ok := reflect.TypeFor[guarded]().MethodByName(name); !ok {
				t.Errorf("%s is not guarded", name)
			}
		}
	}
}

// NFR-REL-002: a prompt that reaches an agent's closed pipe before the connection
// says the agent is gone reports the agent exiting, so the engine starts it again;
// it failed with "write: file already closed" in CI.
func TestAPromptToAnAgentThatJustExitedSaysSo(t *testing.T) {
	for i := 0; i < 5; i++ {
		hs, _ := refAgent(t, "", nil)
		s := hs.(*session)
		s.proc.Stdin().Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := s.Prompt(ctx, harness.Prompt{Text: "hello"})
		cancel()
		if !errors.Is(err, harness.ErrAgentExited) {
			t.Fatalf("try %d: %v", i, err)
		}
	}
}
