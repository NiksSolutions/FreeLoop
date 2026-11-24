---
title: AFK and worktree edges: a failed leftover commit stops the run, a failed start leaves no worktree, an unmerged run is said
status: done
type: afk
blockedBy: []
---

## What
From the review of 2026-11-23 (WFL-006, SES-016); confirm each first.
- **`commitLeftovers` ignores a failed or skipped commit** (`internal/engine/afk.go`): with no `user.name`, `commit.gpgsign` without a key, an index lock, or a blocked config, the run goes on; the next issue's session starts with the previous issue's changes, which end up in its commit and cost.
- **The cleanup after a failed `CreateSession` cannot remove the worktree it made** (`internal/engine/engine.go`, the call to `removeWorktree` with a Session lacking `WorktreeBranch`; `internal/engine/worktree.go` refuses because the worktree is "not on the session's branch"). The folder and branch are orphaned.
- **A second AFK run started before the first run's branch is merged** reads the backlog from the main checkout, where the first run's issues are still open, and does them again (`PrepareAFK`). OQ-040 says the user merges after QA; nothing warns.

## Acceptance
- A failed or skipped leftover commit stops the run with git's message as the reason (WFL-006 says the run commits what the session left).
- A failed session creation removes the worktree and branch it made; a test makes the store fail after the worktree is added.
- `PrepareAFK` names a previous run's branch that is not merged into the project's HEAD as a problem the dialog shows, in en, de and nl.
- Tests for each.

## Notes
- 2026-11-24: Done. `commitLeftovers` (`internal/engine/afk.go`) now returns an error for a failed `git status`, `git add` or `git commit` (git's message), and for a repository whose config names programs (the `blocked` reason, checked before `git status`, which can run a filter); `finishIssue` stops the run with "What <issue>'s session left could not be committed: …" before the review or the next issue. It commits only when something is staged (`git diff --cached --quiet`), so status noise does not stop a run. A failed `CreateSession` now calls `discardWorktree` (`internal/engine/worktree.go`: `git worktree remove --force` and `git branch -D`) for a worktree it made, never for a shared one. `PrepareAFK` sets `AFKPlan.Unmerged` and a Problem when the project's last run with a branch left one that still exists and is not an ancestor of the checkout's HEAD (`merge-base --is-ancestor` through `gitCode`, by `refs/heads/…`); `AFKDialog.tsx` shows it through `planProblem` (`src/afk.ts`, key `afk.dialog.unmerged` in en, de, nl). Tests: `TestLeftoversThatCannotBeCommitted`, `TestAFKRunStopsWhenLeftoversCannotBeCommitted`, `TestAFKPlanNamesAnUnmergedRun`, `TestAFailedCreateSessionRemovesItsWorktree`, and "a run's branch not merged yet is said in each language" in `afk.test.ts`. WFL-005 and WFL-006 in REQUIREMENTS.md say both. Decision taken from the acceptance's word "problem": an unmerged branch blocks the start (Start is disabled, `StartAFK` and a scheduled run refuse) until it is merged or deleted; a softer warning that still lets the run start would be a product change. The Schedules form shows the plan's problem as the engine's English text, as it does for every other problem; it was not changed.
- 2026-11-24: the user chose to keep the refusal: a new run waits until the previous run's branch is merged or deleted.
