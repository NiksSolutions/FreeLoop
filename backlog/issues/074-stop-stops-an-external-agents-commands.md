---
title: Stop stops an external agent's commands, and they show output as they run
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (HRN-005, SES-004). In `internal/harness/acp/acp.go` a `terminal/create` command runs under `context.Background()` and ends only on `terminal/kill`, `terminal/release` or `Close()`: after Stop, an approved command of an agent that does not kill its terminals runs on for up to 10 minutes. And `TerminalOutput` returns nothing until the command has exited, so an agent polling a dev server or a test watch sees nothing.

Also: a tool call cut off mid-arguments vanishes (`internal/harness/builtin/builtin.go:636-639`: the event carries invalid JSON as `rawInput`, `Engine.write` drops it, and the later update refers to a call the window never saw); compaction fails the turn when the model answers the summary request with a tool call (`compact.go:132-138, 203-211`).

## Acceptance
- A turn that ends cancelled stops every terminal the agent started in it. Test with the reference agent: a `sleep` started through a terminal is gone shortly after Stop.
- `terminal/output` returns what the command has printed so far while it runs. Test with the reference agent.
- A cut-off tool call appears in the transcript as a failed call with its raw text. Test with the scripted provider.
- The compaction request offers no tools where the provider allows it, or a tool-call-only answer is retried once without them; it does not fail the turn. Test.

## Notes
- 2026-11-24: `internal/harness/acp/acp.go`: a terminal records the turn it was started in, and a turn whose context is cancelled stops those terminals at once (`stopTerminals`, on Stop and again when the turn ends); one started in an earlier turn runs on until the agent kills or releases it or the session closes, since an agent may read it in a later turn (Claude Code's background shells do), and a `terminal/create` that arrives after the stop is refused. `terminal/output` reads a live buffer (`termOutput`) fed by a new `environment.ExecSpec.Live` writer (Local wires it; Docker and SSH pass it on), so a running command's output shows; past the limit it keeps the latest output, cut at the start on a character boundary as the protocol says (before, it kept the first bytes). `internal/harness/builtin/builtin.go`: a call whose arguments are not valid JSON fails before the broker or an MCP server sees it, and its `tool_call` event carries the raw text as a string, so it is recorded. `compact.go`: the summary request keeps the tools but sets the new `provider.ChatRequest.NoToolCalls` (tool_choice none on Anthropic and Responses, function calling off on Gemini); a tool-call-only answer is retried once with no tools. The reference agent has `WATCH` and `BG` prompts and the scripted provider a `Call.Raw`. Tests: `TestATerminalsOutputShowsWhileItRuns`, `TestStopEndsTheTurnsTerminals`, `TestATerminalsOutputKeepsTheEnd`, `TestCutOffToolCallIsRecorded`, `TestCompactionSurvivesAToolCall`, `TestNoToolCalls` in the three adapters. ARCHITECTURE.md says it. Open for the user: a command an agent means to leave running (a background shell) is stopped too when it was started in the turn that is stopped; whether such a command should outlive Stop is a product question, not decided here.
