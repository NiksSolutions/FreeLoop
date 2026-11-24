---
title: A queued task starts once
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 by two reviewers. `runQueue` (`internal/engine/queue.go:114-165`) starts tasks inside `changeQueue` and saves `queue.json` only afterwards. A crash, a quit or a failed save between leaves the task `waiting`, and it starts again: a second session, worktree and bill. `queueMu` is also held while sessions start. AFK steps already solve this (`Starting`, `recoverStarting` in `afk.go`).

## Acceptance
- A task is saved as starting, with what is needed to find its session again, before its session is started; on load a starting task is reconciled (its session exists: running; not: waiting) and never started twice.
- A failed save after a start does not start the task again on the next tick.
- Sessions start outside `queueMu`.
- Go tests: a crash between start and save (simulated) leaves one session; a failing save starts one session.

## Notes
- 2026-11-24: `runQueue` (`internal/engine/queue.go`) now saves each task it starts as running with `starting: true` and a session ID made beforehand (`session.NewID`), then starts the sessions outside `queueMu` (one pass at a time, `queueRun`), then records the starts. A pass that finds a task still starting looks the ID up: the session is there, it runs; the store does not know it, it waits again; any other error, it holds its place until the next pass. If the first save fails nothing starts. `config.Task.Starting` is `omitempty`, so a beta `queue.json` loads as before (and the Queue page shows a starting task as running, no new state). The store takes a caller's ID in `CreateSession` when it is `NewID`-shaped (`internal/store/store.go`), through the unexported `CreateSessionRequest.id`. Tests: `TestQueueTaskIsSavedAsStartingFirst` (saved as starting with 0 sessions, `queueMu` free during the start), `TestQueueTaskStartsOnceWhenTheSaveAfterFails`, `TestQueueTaskStartedBeforeACrash` (the second engine on the same folder finds the one session; a starting task whose session was never made starts once), `TestQueueFromTheBetaLoads`, `TestCreateSessionWithAnID`. SES-025 in `docs/REQUIREMENTS.md` says so. Left as it was: a crash after the session was made but before its prompt was sent leaves the task running with a session that has no prompt, which holds its queue slot until the user sends the prompt or archives the session; it is not started twice.
