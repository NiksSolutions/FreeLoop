---
title: Agents, MCP and pairing edges: unique made-up call IDs, a changed MCP secret restarts the server, wrong pairing codes do not close the window
status: done
type: afk
blockedBy: []
---

## What
From the review of 2026-11-23 (PRV-006, MCP-005, SEC-020); confirm each first.
- **Made-up tool-call IDs repeat within a session's history** (`internal/provider/openaicompat`, `ollama`, `gemini`: a counter per provider instance, which restarts with the app). After a restart, `call_1_0` appears twice; after a model switch to a provider that requires unique IDs, the request may be refused.
- **A changed MCP secret does not restart the server** (`internal/mcp/manager.go` fingerprints only the count of secrets; `internal/engine/mcp.go` restarts only on a fingerprint or approval change). A rotated key keeps the old one until the user stops the server by hand.
- **Five wrong pairing codes close the pairing window** (`internal/remote/server.go`): any local process (a sandboxed command, another tailnet device once serve is on) can keep the user from ever pairing, without knowing the code.

## Acceptance
- Made-up IDs carry a random part, unique across restarts; a test makes two provider instances and compares.
- Saving an MCP server with changed or removed secrets restarts it (or the fingerprint hashes the values, never storing them); a test.
- Wrong codes are rate-limited instead of closing the window, and the Phone page says when wrong codes were tried; the at-most-five-tries guarantee per code stays (SEC-020). Words in en, de and nl.
- Tests for each.

## Notes
- 2026-11-24: Done. Made-up call IDs: `provider.CallIDs` (`internal/provider/provider.go`) draws a random part once per provider and numbers responses after it; `openaicompat`, `ollama` and `gemini` use it (`call_<random>_<n>_<i>`, `gemini-call-<random>_<n>_<i>`), so a response's IDs stay as they are and two providers, as before and after a restart, never share one; the scripted demo provider is unchanged. Tests: `TestMadeUpIDsDifferAcrossInstances` (openaicompat, ollama) and `TestMadeUpIDsAndBudgets` (gemini, now three responses over two instances). MCP: `Manager.Start`'s in-memory fingerprint adds a SHA-256 of the resolved secret values (`valuesHash`, `internal/mcp/manager.go`), never a value, and `SaveMCPServer` (`internal/engine/mcp.go`) starts a running server again after new secret values are saved, which restarts it only when a value changed; a removed secret already changed the configuration's fingerprint, so the server stops and asks again. Tests: `TestAChangedSecretRestartsTheServer` (internal/mcp), `TestMCPChangedSecretRestartsTheServer` (engine). Pairing (`internal/remote/server.go`): a code still takes at most five wrong tries, then a new code replaces it and the window stays open; after each wrong code no code is looked at (429 with Retry-After) for a pause that doubles from one second to at most a minute, so a guesser can neither close the window nor replace the code faster than it can be typed; `PairingStatus.Wrong` counts the window's wrong codes, which Settings → Phone says (`phone.pair.wrong`), and the phone app says how long to wait (`phone.pair.wait`, `SlowDown` and `retryAfter` in `src/phone/`), in en, de and nl. Tests: `TestWrongPairingCodesDoNotCloseTheWindow` (the old pin that five wrong codes close the window is replaced), the `retryAfter` test in `phone.test.ts`. SEC-020 in REQUIREMENTS.md and SECURITY.md, ARCHITECTURE.md and DEVELOPING.md say what changed. Not verified: a real phone against the pause, and `make check-app` (not mine to run here). A determined flood from something that reaches the port can still win the slot after each pause; the requests cannot be told apart by sender.
