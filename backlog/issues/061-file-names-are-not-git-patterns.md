---
title: A file's name is never read as a git pattern
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23. The engine passes file names to git after `--`, but git still reads glob characters and `:` magic in them. Discarding an untracked file literally named `src/*.bak` also deleted `src/u1.bak` and `src/u2.bak`; a file named `:^nothing` takes every untracked file with it; a done issue named `backlog/issues/0*.md` makes Archive remove open issues. An agent can plant such a name, and the user loses uncommitted work with one click.

## Acceptance
- Every git call the engine makes treats its paths literally (`GIT_LITERAL_PATHSPECS=1` in the git environment, `internal/engine/git.go`), unless a call is shown to need patterns, which then says so.
- A Go test in a scratch repository: Discard on `src/*.bak`, on `:^nothing` and on a tracked, changed `:!zz` touches only that file; stage and unstage of such a name touch only it; Archive with a done issue named `0*.md` leaves the open issues.
- `make test` and `make lint` pass.

## Notes
- Where: `internal/engine/changes.go:338-343, 393-402`, `internal/engine/backlog.go:262-275`.
- 2026-11-23: done. `gitEnv` (`internal/engine/git.go`) sets `GIT_LITERAL_PATHSPECS=1` for every git call the engine makes. Three calls cannot run with it and add `gitPatterns` (`GIT_LITERAL_PATHSPECS=0`) with a comment: the checkpoint snapshot's `git add -A -- . :(exclude)…` (`checkpoints.go`, the exclusions are pathspec magic and the app's own), and both `check-ignore` calls (`snapshotExclusions` in `checkpoints.go` and `ignoredFiles` in `ssh.go`), since check-ignore dies with "pathspec magic not supported by this command: 'literal'"; the SSH one already puts `./` before each name. `gitRunWith` (`changes.go`) and `shadowGitWith` (`checkpoints.go`) carry the extra variables. Tests: `TestFileNamesAreNotGitPatterns` (`changes_test.go`), `TestArchiveTakesNamesLiterally` (`backlog_test.go`), both failing before the fix and skipped on Windows, whose file names cannot hold `*` or `:`. SECURITY.md (the diff panel's git actions) and DEVELOPING.md (a new kind of environment) say so.
