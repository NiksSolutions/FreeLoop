---
title: Rewind does not take another session's work
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (SES-010). Checkpoints are per session but snapshot and restore the whole work root (`internal/engine/checkpoints.go:176-225`). With two sessions in one folder (no worktree, subagents, profile switches), rewinding A resets files B changed, under B's running tools, with nothing in B's transcript.

## Acceptance
- Rewind is refused while another session with the same work root is in a turn, with a message naming it.
- When another session with the same work root has written since the checkpoint, the preview (or the rewind's confirmation, whichever exists) says so, and after the rewind that session gets a notice in its transcript.
- Go test with two sessions in one folder for both cases. Words shown in the window are in en, de and nl.

## Notes
- 2026-11-24, left open: one criterion needs files outside this work's area. Done: `Rewind` (`internal/engine/checkpoints.go`) finds the other unarchived sessions with the same work root (`sharingFolder`) and refuses while one is in a turn (checked before the snapshot and again before the restore), with an error naming it. After the rewind, each one that ran a tool since the checkpoint that may have changed files (any kind but read, search, think, fetch and switch_mode, and only one that started running: a call refused or cancelled before it ran does not count; `changedFilesSince`, read back to the checkpoint with `ScanBack`) gets a `notice` whose new `rewoundBy` field (`event.SessionRef`, `internal/event/event.go`) names the session that rewound; `transcript.ts` shows it as `transcript.rewoundBy` in en, de and nl, and any other notice as given. `Engine.RewindPreview(ctx, sid, seq)` returns those sessions (`others`) and one in a turn (`busy`). Tests: `TestRewindInASharedFolder` (three sessions in one folder: refused beside a turn, naming it, files untouched; preview lists the one that wrote; after the rewind only that one has the notice), a `transcript.test.ts` case for the notice in English and German. SES-014 in `docs/REQUIREMENTS.md` and the help's `sessions.md` (en, de, nl) say so.
- Not done: the preview does not reach the window yet. Showing it in the rewind's confirmation (`MessageActions` and the palette's rewind, both in `SessionView.tsx`) needs a `RewindPreview` method on `EngineService` in `cmd/freeloop/service.go` and the confirmation calling it and naming `others` (and `busy`), with words in en, de and nl. Both files belonged to other agents this time.
- For a decision: the refusal is an engine error, which GUI-048 shows as given, in English, like the rewind's other errors ("stop the running turn before rewinding"). Showing it in de and nl would need a structured error the window knows (or the confirmation refusing beforehand from `busy`), which goes against GUI-048's "engine error details stay English".
- Found on the way, not fixed (outside this work's area): a rewind to before a built-in session's first message writes `history_message` `{"index":0,"reset":true,"clear":true}` (`TruncateHistory(0)`, `internal/harness/builtin/builtin.go`), which `schemas/events.json` refuses (`historyMessage` requires `message`), so under `FREELOOP_CHECK_EVENTS` it panics. It is so at HEAD; `TestRewindInASharedFolder` rewinds to a later checkpoint, as `TestCheckpointsAndRewind` does.
- 2026-11-24: the preview reaches the window. `EngineService.RewindPreview` (`cmd/freeloop/service.go`); `rewindCheck` in `frontend/src/rewind.ts` (tested) is asked by `rewind` in `SessionView.tsx` before either way of rewinding: beside another session's turn it says so in the window's language (`session.rewindBusy`) and does nothing; when other sessions changed files since, it names them and asks (`session.rewindOthers`). The engine still refuses beside a turn whatever the window found. Left: a rewind of a built-in session to before its first message writes a `history_message` event the schema rejects (found here, not fixed).
