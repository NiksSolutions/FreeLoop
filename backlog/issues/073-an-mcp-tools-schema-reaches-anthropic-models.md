---
title: An MCP tool's whole schema reaches an Anthropic model
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (MCP-002, PRV-001). `internal/provider/anthropic/anthropic.go:129-138` copies only `properties`, and `required` only when it is a Go `[]string`. MCP schemas come from `json.Unmarshal`, so `required` is `[]any` and is dropped, with `$defs`, `additionalProperties`, `oneOf` and the rest.

Also from the review, small and in the same area:
- `internal/provider/openairesponses/openairesponses.go:188, 226` ignores `IsError`; the other adapters prefix "Error: ".
- `internal/provider/ollama/ollama.go:364` returns a plain error on an early stream end instead of `provider.ErrStreamEnded`, so the retry never applies.
- `internal/engine/mcp.go:349-356`: two tools of one server can map to one name (`read.file`, `read_file`).

## Acceptance
- `required` as `[]any` is passed, and the schema's other keys reach the request (the SDK's extra fields). Test with a schema using `required`, `$defs` and `additionalProperties`.
- The Responses adapter marks a failed tool result as the others do; Ollama returns `ErrStreamEnded`; colliding MCP tool names get a distinguishing suffix and both stay callable. A test each.

## Notes
- 2026-11-24: `internal/provider/anthropic/anthropic.go` (`inputSchema`) sends the whole schema: `required` as `[]string` or `[]any`, every other keyword as the SDK's `ToolInputSchemaParam.ExtraFields` (v1.84.0 marshals them with `param.MarshalWithExtras`); built-in schemas go out byte for byte as before. `openairesponses.go` (`toolOutput`) prefixes a failed result with "Error: " on both of its paths; `ollama.go` returns `provider.ErrStreamEnded` on an early end, so PRV-010's retry applies. `internal/engine/mcp.go` (`mcpToolNames`): of two tools of one server with the same safe name, one whose name needed no change keeps it and the others get `_` and eight hex digits of their own name's SHA-256, so both stay callable and the names do not depend on the order the server lists them in. Tests: `TestWholeToolSchemaIsSent`, `TestFailedToolResultIsMarked`, `TestEarlyStreamEndIsRetried`, `TestMCPToolNameCollision` (both tools called through a real stdio server; an allow rule on one and a deny rule on the other each land on their own tool). ARCHITECTURE.md says it.
