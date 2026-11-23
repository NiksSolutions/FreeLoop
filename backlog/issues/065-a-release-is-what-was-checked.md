---
title: A release is built from what was checked, and the gates cannot pass by accident
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (SEC-012, SEC-021, NFR-SEC-004).
- `scripts/verify-repro.sh:30-44`: the builds run in a subshell left of `||`, where `set -e` is off, so a failed Mac build still reports "reproducible" over the two Windows exes.
- `.github/workflows/ci.yml`: `release` needs only `build`, `package-windows` and `package-linux`, not `test` and `windows`.
- The `release` and packaging jobs restore the same Go cache that the job running third-party agent CLIs (`make verify-agents`) saves, and `release` never runs `make verify-deps`.
- The update-signing key is used in the job that ran `npm ci` and the build, and a dry run signs with the real key from any ref.
- `scripts/memcheck.sh` passes at 0 MB when measuring fails.
- The release notes still say goose is not pinned.

## Acceptance
- `verify-repro.sh` fails when either build fails or an expected file is missing; shown by running its build function against a failing make in a scratch folder.
- `release` needs `test` and `windows` too; the release and packaging jobs use no shared Go cache and run `make verify-deps`; verify-agents runs in a job of its own.
- Signing happens in a job of its own that only checks out, downloads the built files and runs `freeloop-sign`; the key is passed only on a `v*` tag. A protected environment with a required reviewer is a repository setting: leave a note for the user, do not change settings.
- `memcheck.sh` fails on an empty or zero measurement.
- The release notes say what is pinned. DEVELOPING.md and SECURITY.md match the workflow.
- The workflow parses (`actionlint` if present, else a YAML load); every action stays pinned to a commit.

## Notes
- 2026-11-23: done. Each finding confirmed by reading; the two scripts' also reproduced. `scripts/verify-repro.sh`: each `make` checked on its own (`run_make`) and every compared file required in both builds; run with a stand-in `make` in a scratch folder under /tmp (a path with a space, sh and dash), the old script said "reproducible: 2 files match" over a failed Mac build, the new one fails on a failed Mac or Windows build and on a missing bill or exe, still reports a real difference, and passes when both builds succeed. `.github/workflows/ci.yml`: `release` needs `test`, `windows`, `build`, `agents` and both package jobs; the real agents moved from `build` to an `agents` job (weekly, on tags and in a dry run; no cache, no token kept), and `build` no longer runs weekly; `release`, the package jobs and `sign` turn off setup-go's and setup-node's caches, and those that build run `make verify-deps` first; `release` no longer signs or writes (contents: read) and keeps its files as the artifact `release-<version>`; a `sign` job (contents: read) only checks out, downloads that artifact, builds `freeloop-sign` and signs and verifies, with `RELEASE_SIGNING_KEY` in that one step's environment only for a `v*` tag that is not a dry run, and keeps `SHA256SUMS.sig`; a `publish` job (contents: write, the only one) checks that the folder holds exactly the files `SHA256SUMS` lists plus the list and its signature, then makes the draft. No actionlint here, so the workflow was checked by loading it (PyYAML, Ruby) and a script: every `needs` names a job, outputs read are declared, every action is pinned to a commit the file already used, the key appears once, and the publish check and the merge step ran on stand-in folders. It has not run on GitHub. `scripts/memcheck.sh`: `mb` prints a total only when every process given was measured, and an empty or zero result, or no WebKit process started with the app, fails; tested with a stand-in `footprint` and against the real one, without running the memcheck. `docs/release-notes/0.1.0-beta.1.md` now says goose runs the version the user installed and is checked at 1.57.0, pinned by checksum, on Apple Silicon only, and that this beta's tag run predates the pin. `docs/DEVELOPING.md` (making a release, CI's jobs, signing), `docs/SECURITY.md` (threat 13 and a section for this review), SEC-021 in `docs/REQUIREMENTS.md` and the SEC-021 line of `docs/ROADMAP.md` match.
- For the owner, by hand: (1) in Settings → Environments make `release` with a required reviewer and deployments limited to `v*` tags, move `RELEASE_SIGNING_KEY` into it as an environment secret and delete the repository secret, then add `environment: release` to the `sign` job, where a comment marks the place; (2) a tag ruleset limiting who may push `v*` tags; (3) watch the first dry run (`gh workflow run CI -f dry_run=true`): the `agents` job now runs in it, `sign` should print NOT SIGNED and pass, and `release-<version>` should hold the files; then the first tag: `sign` must verify, and `publish` must find all files and the signature and make the draft. Not changed: the Developer ID secrets are still taken in the release job, which runs `npm ci` and the build; when a Developer ID exists (OQ-057) they want the same protected environment and, best, a job of their own.
