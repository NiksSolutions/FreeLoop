---
title: After a power loss a session is not left in a turn
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (NFR-REL-001; the store half confirmed by test). The transcript is fsynced every 200 ms; SQLite runs WAL with `synchronous(NORMAL)` (`internal/store/recover.go:60`), so after a power loss a row can say `idle` while the transcript ends in `turn_started` / `session_status running` / `permission_requested`. `recover` (`internal/engine/runtime.go:887-895`) picks sessions by row status only, and `repairRow` does not replay status, so the turn is never closed.

## Acceptance
- At start, a session whose transcript ends inside a turn is recovered whatever its row says: the turn is ended, open permission requests are decided as cancelled, status is idle, as for a row that said running. The cost at start stays bounded (do not read every transcript in full: use the tail).
- Go test: start a turn, roll the row back by hand, reopen: the transcript ends with the closing events and the row is idle.

## Notes
- 2026-11-24: `recover` (`internal/engine/runtime.go`) no longer picks sessions by their row: it reads every session's transcript from its end with `ScanBack` (`readTail`) and recovers one whose row is busy, whose transcript ends inside a turn, or whose last status is busy or was lost after `turn_ended`. Open requests of the last turn are decided as cancelled, the turn ended, and the status set idle (an archived row stays archived). The scan stops at the last status when it is of a session at rest, else at the last turn's start or end, or at an import's end, so a session at rest costs one read of its file's end and is not written to, a recovered one ends at rest so a second start does nothing, and a busy row is no longer read from its first line (the `DEVELOPING.md` table loses its `recover` row). The store is unchanged: `repairRow` still leaves the status alone, and the engine writes the closing events. Tests: `TestRecoverClosesATurnTheRowLost` (turn waiting on a permission, crash, row rolled back by hand to idle and an older `last_seq`, reopen: `permission_decided,turn_ended,session_status`, row idle, the session at rest untouched, a second start writes nothing), `TestRecoverReadsTheTranscriptsEnds` (no whole read; long sessions at rest and in error untouched; only the undecided request is cancelled). NFR-REL-001 in `docs/REQUIREMENTS.md` and "Reading a transcript" in `docs/DEVELOPING.md` say so.
