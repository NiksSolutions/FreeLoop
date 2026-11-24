---
title: Session runtime edges: Stop during the checkpoint, prompts the idle stop meets, a dead agent's runtime, queued prompts on quit
status: done
type: afk
blockedBy: []
---

## What
From the review of 2026-11-23 (SES-004, NFR-REL-001, NFR-REL-002); confirm each first. Line numbers are from cd40672 and have moved.
- **The pre-turn checkpoint ignores Stop** (`internal/engine/runtime.go` `run`, `internal/engine/checkpoints.go`): `e.checkpoint(r)` runs under `context.Background()` with a 150 s cap before `turn_started`. In a large folder Stop writes "You stopped the turn." but the turn starts afterwards; archive, delete and quit wait on it; the session shows idle meanwhile.
- **A runtime whose agent exited stays usable for a moment** (`run`, the `ErrAgentExited` branch): `stopped` is not set, and the deferred `stopRuntime(sid)` goes by session ID, so it can stop a fresh runtime installed meanwhile. Related: issue cafa47b's race; the engine test `TestAnExternalAgentIsStartedAgainAfterItExits` failed in CI once.
- **A prompt that meets the idle stop is refused** (`enqueue` returns `errStopped`, "send it again"); scheduled, AFK and phone prompts have nobody to resend.
- **Queued prompts vanish without a record** on quit, crash, archive or Stop (`r.queue = nil`); only the agent-exited and sign-in paths say how many were dropped.
- **`enqueue` writes `session_status error` for any start failure**, `errClosing` at quit included.

## Acceptance
- The checkpoint runs under the turn's context: Stop during it ends the turn at once, and the session shows it is starting (or the user message is written) before the snapshot.
- The exited runtime is marked stopped under its lock, and the deferred stop stops only that runtime.
- A prompt that meets a stopped runtime gets a new one, once, unless the engine is closing.
- Dropped queued prompts are said in the session (how many, and why) wherever they are dropped.
- A start that fails because the app is closing leaves the status as it was.
- A Go test for each, run with `-race`.

## Notes
- 2026-11-24: Left open for one decision; everything else is built. `internal/engine/checkpoints.go`: `checkpoint` runs under the turn's context and marks the session `running` before the snapshot; Stop kills git, and `run` (`internal/engine/runtime.go`) records the turn as an agent stopped at once would, without the agent getting the message (`stoppedBeforeStart`: `turn_started`, the message, the stop's notice after it, a cancelled `turn_ended`, `idle`), so queued tasks and AFK steps see the turn end. The stop's notice waits for the message (`runtime.starting`, `stopWhy`). `snapshot` clears the lock files a killed git leaves in the store (`clearLocks`: `index.lock`, `HEAD.lock`, the branch's lock), so the next checkpoint works, after a kill of the app too. `frontend/src/transcript.ts` counts a `running` status as running and a status at rest as not, so the window offers Stop during the snapshot. The exited agent's runtime is marked `stopped` (and `replace`) under its lock before the notice, and the deferred stop is `retire(r)`, which stops only that runtime and leaves one started since (`stopRuntimeOf`, `internal/engine/sessions.go`); `runtime.stop` runs its closing once (`halted`), so a second stopper waits for the first. A prompt that meets a runtime stopped by the idle stop or an exited agent retires it and starts a new one, once (`enqueue`, `start`); not while the app quits (`errClosing`), and not for a runtime stopped for an archive, a delete or a subagent's end, which still answers "send it again", since those end the session's work (a judgement; the issue said any stopped runtime). `runtimeFor` refuses a new start while the app quits, and `startFailed` records no error or status for that. Dropped queued prompts are said with how many: Stop, archive, quit, "continue in another session" and a parent's stop through `stopTurn` ("You stopped the turn. 2 queued messages were not sent."), a stop of the runtime itself through `stopRuntimeOf` ("The session stopped, so 1 queued message was not sent."), the exited agent's notice now in the singular too. Tests (`internal/engine/runtime_edges_test.go`): `TestStopDuringTheCheckpointEndsTheTurnAtOnce`, `TestAnExitedAgentsRuntimeIsStoppedAtOnce`, `TestAPromptThatMeetsTheIdleStopStartsANewRuntime`, `TestAStartRefusedAtQuitLeavesTheStatus`, `TestDroppedQueuedMessagesAreSaid`; frontend "a session running before its turn starts shows as running". ARCHITECTURE.md (SES-014) says it. **Open:** queued prompts lost to a crash are still not said: the queue lives only in memory, and saying it at the next start needs the queue recorded, as a new transcript event (`schemas/events.json`, every change of the queue written) or a column of the session's row. Which, and whether the window should then show the queue from it, is a decision for the user.
- 2026-11-24: the user chose to leave the one point still open: messages queued at the moment of a crash are not saved, so the next start cannot say they were lost. Rare, and they are never sent twice; every other drop is said in the session.
