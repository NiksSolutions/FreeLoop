// Writes a session for the phone's check (issue 057) as a transcript: what an agent
// writes that is hard to fit on a phone, a wide table, long code lines, long commands
// and paths, a link and a word with no break, beside short messages.
//   node scripts/appcheck/phonesession.ts <sid> <events.jsonl>
import { writeFileSync } from "node:fs";

const [sid, out] = process.argv.slice(2);
const events: object[] = [];
const t0 = Date.parse("2026-11-22T09:00:00Z");
const emit = (type: string, data: unknown) => {
  const seq = events.length + 1;
  events.push({ v: 1, seq, ts: new Date(t0 + seq * 1000).toISOString(), sid, type, data });
};
const text = (type: string, t: string, extra: object = {}) => emit(type, { messageId: "msg_phone", content: { type: "text", text: t }, ...extra });
const tool = (id: string, title: string, kind: string, status: string, command?: string, output?: string) => {
  emit("tool_call", { toolCallId: id, title, kind, tool: kind === "execute" ? "shell" : kind === "read" ? "read_file" : "edit_file", status: "pending", rawInput: command ? { command } : {} });
  if (status !== "pending") emit("tool_call_update", { toolCallId: id, status, ...(output ? { content: [{ type: "content", content: { type: "text", text: output } }] } : {}) });
};

emit("session_status", { status: "idle" });
emit("turn_started", {});
text("user_message_chunk", "Why does the build fail on the CI runner? Fix it if you can.");
emit("session_status", { status: "running" });
text(
  "agent_message_chunk",
  [
    "## What I found",
    "",
    "The test `internal/engine/limit_test.go` waits on `TestProviderConcurrencyCap_withAVeryLongNameThatDoesNotBreak` and the runner is slower than this Mac.",
    "",
    "- The cap is checked with a fixed sleep.",
    "- The Windows runner takes longer, so the third call starts late.",
    "",
    "| Job | Runner | Result | Time | Attempts | Notes |",
    "| --- | --- | --- | --- | --- | --- |",
    "| test | ubuntu-latest | passed | 4m 12s | 1 | nothing to report here at all |",
    "| windows | windows-2025-arm64 | failed | 11m 48s | 3 | TestProviderConcurrencyCap timed out |",
    "",
    "```go",
    "func TestProviderConcurrencyCap(t *testing.T) { e := newTestEngine(t, withProvider(slowProvider{gate: make(chan struct{})})) // the gate opens when three are queued",
    "\tdefer e.Close()",
    "}",
    "```",
    "",
    "See https://github.com/NiksSolutions/FreeLoop/actions/runs/45049646063/job/123456789012?pr=1&check_suite_focus=true for the log.",
    "",
    "averyveryverylongwordwithoutanybreakwhatsoeverthatkeepsgoingandgoingpastthescreenedge",
  ].join("\n"),
);
tool("toolu_phone_1", "Run go test ./internal/engine/ -run '^TestProviderConcurrencyCap$' -count=50 -race -timeout 600s", "execute", "completed", "go test ./internal/engine/ -run '^TestProviderConcurrencyCap$' -count=50 -race -timeout 600s", "ok  \tgithub.com/FinanceNik/Freeloop/internal/engine\t48.213s");
tool("toolu_phone_2", "Read /Users/someone/Library/Application Support/freeloop-worktrees/freeloop/afk-001-language-setting/internal/engine/limit_test.go", "read", "failed");
tool("toolu_phone_3", "Edit internal/engine/limit_test.go", "edit", "completed");
// A token with no break anywhere, a hash such as real commands and their output carry:
// in a grid of the old layout, one widened the whole conversation past the screen.
const hash = "ae216c2ef5247a3782c135efa279a3e4cdc61094270f5d2be58c6204b7a612c9";
tool("toolu_phone_4", `Run printf %s ${hash}${hash} | shasum -a 256`, "execute", "completed", `printf %s ${hash}${hash} | shasum -a 256`, `${hash}  -`);
emit("notice", { message: `Saved the output to /tmp/${hash}${hash}.output` });
emit("error", { message: `The hook failed: ${hash}${hash}` });
text("agent_message_chunk", `The digest is ${hash}${hash}, and \`${hash}${hash}\` in code.\n\n`);
text("agent_message_chunk", "Fixed: the test now waits on a gate instead of a sleep.");
emit("turn_ended", { stopReason: "end_turn" });
emit("session_status", { status: "idle" });
emit("turn_started", {});
text("user_message_chunk", `Thanks. Push it when CI is green. ${"f".repeat(90)}`, { via: "phone:Check iPhone" });
emit("session_status", { status: "running" });
text("agent_message_chunk", "Will do.");
emit("turn_ended", { stopReason: "end_turn" });
emit("session_status", { status: "idle" });

writeFileSync(out, events.map((e) => JSON.stringify(e)).join("\n") + "\n");
