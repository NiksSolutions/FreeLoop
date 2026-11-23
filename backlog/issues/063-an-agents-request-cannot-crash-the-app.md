---
title: An external agent's request cannot crash the app
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (HRN-002). `fs/read_text_file` with a huge `limit` overflows `start+*r.Limit` (`internal/harness/acp/acp.go:963-966`), the slice panics, and the protocol library runs these handlers in goroutines with no recover, so every session dies with the app. The same agent's `outputByteLimit` for a terminal is used with no ceiling (`acp.go:1051-1053`).

## Acceptance
- A read with `line` 2 and `limit` max int64 returns the file's lines from 2, and a negative or zero limit behaves as today.
- A panic in any handler the client answers an agent with becomes an error to that agent and a logged line, not the end of the process.
- `outputByteLimit` is capped (1 MB).
- Go tests for all three with the reference agent in `acp/acptest`. `make test` and `make lint` pass.

## Notes
- 2026-11-23: done. `internal/harness/acp/acp.go`: the client is handed to the SDK as `guarded{s}`, whose every handler (the eight of `sdk.Client`, `UnstableCreateElicitation` and `HandleExtensionMethod`) defers `recovered`, which logs the panic with its stack and answers the agent "the client failed on <method>"; the goroutine running a terminal's command recovers the same way. `ReadTextFile` compares the limit with the lines left (`*r.Limit < end-start`), so line 2 with limit max int64 returns lines 2 on, and a zero or negative limit still reads to the end. `outputByteLimit` is capped at `maxTerminalOutput` (1 MB). The reference agent (`acp/acptest`) takes "READ <path> <line> <limit>" and "RUN <outputByteLimit> <command>". Tests: `TestAReadsLimitCannotOverflow` (killed the whole test binary before the fix), `TestATerminalsOutputLimitIsCapped` (3000000 bytes came back before), `TestAPanicInAHandlerIsAnError` (an environment that panics in Resolve and Exec; panicked the test binary before), `TestEveryHandlerIsGuarded` (a handler the session has but the wrapper lacks would silently not be offered). ARCHITECTURE.md (the ACP client as built) says so. The requirement is HRN-005 (generic ACP client) with NFR-REL-002, not HRN-002 (the built-in harness). Not changed: an error from the terminal's `Exec`, and so a recovered panic there, still reaches the agent only as empty output with exit code 0, as before.
