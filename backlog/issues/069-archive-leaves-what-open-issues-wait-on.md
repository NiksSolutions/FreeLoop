---
title: Archiving finished issues leaves what open issues wait on
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (WFL-004). `ArchiveBacklog` (`internal/engine/backlog.go:248-262`) removes every done issue; `Analyze` (`internal/backlog/backlog.go:236-239`) counts a missing blocker as waiting. An open issue with `blockedBy: [001]` waits forever once `001` is archived, and AFK runs skip it.

## Acceptance
- Archive leaves a done issue that an open issue still names in `blockedBy`, and its result says how many were kept and why. A missing blocker still blocks (a typo must not unblock).
- Go test: 002 blocked by done 001; after Archive, 001 is still there and 002 is ready.

## Notes
- 2026-11-24: `backlog.Archivable` (`internal/backlog/backlog.go`) splits the done issues into those archive may take and those an open issue still names in `blockedBy`; `ArchiveBacklog` and the Board's "Archive finished" count (`internal/engine/backlog.go`) use it, so a kept issue is not removed or counted, its path goes into the result's `Left` with which open issues wait on it, the cleanup stage's message says how many were left, and with only kept issues finished the error names them. `Analyze` is unchanged: a missing blocker still blocks. Tests: `TestArchivableKeepsWhatOpenIssuesWaitOn`, `TestArchiveKeepsWhatOpenIssuesWaitOn`; `TestArchiveBacklog` now archives an issue nothing waits on (it had asserted the defect). WFL-004 in `docs/REQUIREMENTS.md` says so.
