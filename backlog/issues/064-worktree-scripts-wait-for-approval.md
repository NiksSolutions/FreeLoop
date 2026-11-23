---
title: A project's changed worktree scripts wait for approval, as its hooks do
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (SEC-003; see how project hooks are pinned in `internal/engine/projecthooks.go` and `internal/config/hooks.go`). In a trusted project, `.freeloop/worktree.yaml`'s `setup`, `run` and `archive` are read fresh from the checkout and run with the environment's variables and secrets (`internal/engine/worktreescripts.go:108-128, 254`). A pull or a branch switch that changes them runs the new commands on the next worktree session, unasked; the same change to `hooks.yaml` waits in Settings.

## Acceptance
- The project's worktree scripts are part of what the user approves: while the file differs from what was approved, none of its commands run, the session says why, and Settings shows the change to approve, in the same place and words as changed hooks.
- A project trusted before this change is asked once.
- The `copy:` list is treated as the reviewer of the code finds right; say which in the notes.
- Go tests: scripts changed after approval do not run until approved; approving runs them. SECURITY.md says so. The words are in en, de and nl.
- `make test`, `make lint` and `make check-app` pass.

## Notes
- 2026-11-23: done. Confirmed by reading at cd40672: `ProjectScripts` merged the checkout's `.freeloop/worktree.yaml` into what runs whenever the project was trusted, with no mark. Trusting now approves the file as one set with the hooks' mechanism (`config.ProjectWorktreeFingerprint`, an `approvalMark` in `commandApprovals`): `SetProjectTrust` calls `approveProjectCommands`/`revokeProjectCommands` (`internal/engine/projecthooks.go`, which approve and end the hooks' and the scripts' marks together). While the file differs from what was approved, `ProjectScripts` applies none of it (`RepoWaiting`), so no setup, run or archive command of the file runs and nothing is copied; scripts set in the app still apply. `scriptsFor` puts a notice in the session naming the file and what it would run, `RunWorktreeScript` says the run command waits for approval, and the new-session form's worktree scripts show a banner (`worktreeScripts.waiting`). Settings lists it under "Commands that would run" as "A trusted project's changed worktree scripts" (`project_worktree`, `pending.ts`, en/de/nl), it is among the problems the app shows, and approving it there applies it. A project trusted before this change has no mark: its file waits once and is not adopted on start (unlike the hooks, a pull may already have changed it). The `copy:` list is part of the approved set: the trust prompt shows it, and a changed list hands the agent, and a container that cannot see the main checkout, files the repository ignores; reading one by a secret's name still asks. Tests: `TestChangedWorktreeScriptsWaitForApproval` and `TestWorktreeScriptsOfAnEarlierTrustAreAskedOnce` (`internal/engine/worktreescripts_test.go`), both failing before the fix, `TestProjectWorktreeFingerprint` (`internal/config/worktree_test.go`), `pending.test.ts`; `TestScriptsRunInTheContainer` now approves the file it changes after trusting. SECURITY.md (the hooks bullet and a section), SES-016 in REQUIREMENTS.md, ARCHITECTURE.md's SES-016 note and the permissions help (three languages) say so. No app-check step: the check's projects have no worktree.yaml and adding one needs `scripts/appcheck.sh`, which another agent was editing.
