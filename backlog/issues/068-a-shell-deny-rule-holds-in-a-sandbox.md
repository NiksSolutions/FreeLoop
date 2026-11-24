---
title: A shell deny rule holds in a sandbox and against an alias
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (SEC-003; confirmed there by probes against `Broker.evaluate`).
- `internal/permission/broker.go:446-453`: a sandbox-contained command is auto-allowed unless the line is not plain; it ignores `unseen` (`hidesFromDenyRules`), which bypass mode honours. With `deny shell(curl *)`: `cur? x`, `/usr/bin/cur* x`, `echo curl x | sh`, `env -S 'curl x'`, `repeat 1 curl x`, `busybox sh -c 'curl x'`, `find . -name x -exec curl y +` are "allowed by sandbox".
- `broker.go:428-429`: plan mode in a sandbox (`ContainedReadOnly`) allows before even the not-plain check: `c=curl; $c http://localhost:1`.
- `internal/permission/rules.go:443-448, 519`: with `allow shell(git *)` and `deny shell(git push *)`, `git pu*h` and `git -c alias.p=push p origin` are "allowed by rule".
- `rules.go:249, 404`: `time -p curl x` hides `curl`, because `time` is matched as a keyword before it is a wrapper.
- In bypass mode these pass `deny shell(git push *)`: a leading redirection (`</dev/null git push`, `2>&1 git push`), `bash -O extglob -c 'git push'`, `env -S 'git push'`, `coproc git push`, `echo 'git push' | sh`, `sh <<< 'git push'`.

## Acceptance
- In a sandbox, ask and plan mode both ask (never auto-allow) when a shell deny rule exists and the line is unseen or not plain.
- A glob in the subcommand word, `git -c alias.*=` and `--config-env` count as unseen for a program a deny rule names; `time` with options shows its command; leading redirections are stripped; a shell reading its commands from input counts as unseen; `coproc`, `repeat` and `-` are handled.
- Table tests with every spelling above, in ask, plan, bypass and sandboxed modes; lines with no deny rule in play behave as before (no new prompts for plain commands).
- SECURITY.md keeps saying shell deny rules guard against mistakes, and lists what is now caught.

## Notes
- 2026-11-24: done. `Broker.decide` (`internal/permission/broker.go`) computes `hidden`, which is `unseen` (`hidesFromDenyRules`, as bypass uses it) or a line that is not `plain` while a shell deny rule exists (`shellDenyRule`, which now also counts a rule whose tool is a pattern matching shell); the sandbox in ask and accept-edits mode and plan mode's read-only sandbox ask on it instead of allowing. In `internal/permission/rules.go`, `commandsOf` takes redirections out wherever they stand (`withoutRedirections`; `segments` no longer splits the `&` of `2>&1`), reads a shell's options and finds what `-c` (fish's `--command`, `-C`) runs or that it reads its input (`shellLine`, replacing the `shellC` regex), reads `env -S` (`envSplit`) and wrapper options joined in one word (`-iu NAME`), treats `time` as a wrapper rather than a keyword, unwraps `coproc`, zsh's `-` and `repeat`, `busybox` and `toybox`, follows `find -exec` and process substitution, reads a function's body, and marks as unseen: an alias defined on the line, `git -c alias.*`/`include*`, `--config-env` and `GIT_CONFIG_*` before git (`gitSubcommand`, which `withoutGitOptions` now uses), a glob or brace in the subcommand word when it may spell the deny rule's word (`mayExpand`: `pu*h` may be `push`, `*` may be `-rf`, `*.o` neither), a brace in the program, `env -S` with an escape, `{}` as find's program, and cmd or PowerShell with no command (they read their input). Also found and fixed: a deny rule naming a wrapper (`deny shell(sudo *)`) matched nothing; every wrapper now gives the rules its own line, up to 16 per command, past which the line is unseen (keeps a long chain linear; `find -exec` is followed once per action for the same reason). The bypass refusal now reads "the command builds, renames or reads from its input what it runs, …". Tests: `internal/permission/hiding_test.go` (`TestAShellDenyRuleHoldsWhereNothingElseAsks`: every spelling above and more, in ask, accept-edits, plan and bypass, uncontained and in both sandboxes, and the same lines with no deny rule running as before; `TestLinesThatHideNothingStillRun`; `TestADenyRuleOnAWrapperHolds`; `TestLongChainsStayCheap`). Docs: SECURITY.md (the shell paragraph of the limitations, the sandbox paragraph, issue 035's "still not seen", and a section for this issue), ARCHITECTURE.md (evaluation order). Still not seen, and said so in SECURITY.md: an alias in a config file git is pointed at (`HOME=. git p`, `--git-dir`), `case` patterns in bypass mode, a wrapper's long option with a separate value.
