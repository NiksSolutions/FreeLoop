---
title: A rule written with ./ matches
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23. Claude Code's own documentation writes deny rules as `Read(./.env)`. The import (`internal/engine/claudeimport.go:330-340`) passes `./x` through unchanged, and the matcher compares path segments, so the imported rule matches nothing and the user believes `.env` is protected.

## Acceptance
- The import turns `./x` into `x`; a hand-written rule path with a leading `./` matches the same files (normalised where rules are validated or matched).
- Go tests: `Read(./.env)` and `Read(./secrets/**)` imported deny reads of `.env` and `secrets/a/b`; a rule already saved with `./` matches after an update.
- `make test` and `make lint` pass.

## Notes
- 2026-11-23: done. `permission.TrimDotSlash` (`internal/permission/rules.go`) drops a leading `./` (repeated, `.//./x` too; `./` alone stays, so it never becomes a rule for every path). It is applied where rules are matched: `Rule.matchPath` (the broker, CheckPolicy, search hiding, the SSH sync's deny check) and `DeniedPathPatterns` (the sandbox's regular expressions, whose `^root/\./…` also matched nothing, literal paths included). The import (`convertRule` in `internal/engine/claudeimport.go`) writes `Read(./.env)` as `read_file(.env)`. A rule already in rules.yaml or a project's `.freeloop/rules.yaml` with `./` is left as written and matches. Tests: `TestARulePathWithDotSlashMatches` (`internal/permission/rules_test.go`, broker and sandbox patterns, a Windows `.\secrets\**` row), `TestImportedDotSlashRulesMatch` (`internal/engine/claudeimport_test.go`: the import's destinations, then CheckPolicy in bypass denies `.env`, `secrets/a/b` and, from a saved `./private/**` rule, `private/key`), both failing before the fix. ARCHITECTURE.md (the rule's path, the import) says so.
