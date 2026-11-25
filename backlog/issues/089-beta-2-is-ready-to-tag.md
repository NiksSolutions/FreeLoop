---
title: 0.1.0-beta.2 is ready to tag
status: done
type: afk
blockedBy: []
---

## What
The release itself, after issues 078 to 088.

## Acceptance
- The version is 0.1.0-beta.2 everywhere the version test ties together (`AppVersion`, `build/config.yml`, the plists, the Windows and Linux packaging) and wherever else it is written (README, help, release notes file names).
- `docs/DEVELOPING.md` ("Making a release") gives the owner the exact steps for the protected `release` environment (required reviewer, `v*` tags only, `RELEASE_SIGNING_KEY` moved into it, `environment: release` on the sign job) and a tag ruleset, and the order: dry run, read the draft, tag.
- Nothing is tagged, published or changed in the repository's settings.

## Notes
- 2026-11-25: the version and the release steps are done; left open until 085 to 088 and the release notes are in, which "ready to tag" waits for. Version 0.1.0-beta.2 in `AppVersion` (`internal/engine/updates.go`), `build/config.yml`, `frontend/package.json` and both `"version"` lines of its lockfile (by hand: npm 10 here would also drop the `libc` fields npm 11 wrote), the NSIS header comment and the README (status line, "Download a release"; the clone address there now names `NiksSolutions/FreeLoop`, not the old account anyone could take). The plists stay `0.1.0`; the Windows and Linux packages read `build/config.yml`; the help names no version. `TestAppVersionMatchesTheBuild` now also counts the lockfile's two version lines. `docs/DEVELOPING.md`, "Making a release": the owner's one-time steps (the `release` environment, the key moved, the `sign` job naming it, a tag ruleset) and each release's order (dry run, read the artifact, tag that commit, approve the signing, read the draft, publish). The sign job runs in a dry run too, so a plain `environment: release` would make a dry run from `main` fail on the `v*` rule; the steps give a conditional name instead, whose empty case GitHub does not document, with the next dry run as its check and a fallback. The draft exists only after the tag, so the order is dry run, tag, read the draft, not "read the draft, tag". Nothing tagged, pushed or changed in the repository's settings.
- 2026-11-25 (coordinator): issues 078 to 088 are in, the release notes are final, and make lint, make test and make check-app pass. What remains is the owner's: the `release` environment, then the dry run and the tag, as DEVELOPING.md's "Making a release" says.
