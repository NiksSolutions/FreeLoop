---
title: A tester can check a download's signature with the files they have
status: done
type: afk
blockedBy: []
---

## What
From the review of 2026-11-23 (SEC-021). `freeloop-sign verify` opens every file listed in `SHA256SUMS` and fails on the first one absent (`cmd/freeloop-sign/main.go`), so a tester with only the DMG, `SHA256SUMS` and `SHA256SUMS.sig` cannot check the project's signature, though `docs/SECURITY.md` says anyone can. The README's check (`shasum -c SHA256SUMS --ignore-missing`) proves integrity against a file from the same place, not who made it. The reproducibility claim in the docs is wider than `verify-repro` tests (it compares two builds on one machine; the published files are built in another job).

## Acceptance
- `freeloop-sign verify` checks the signature over the whole list and the files present, with a flag to skip absent ones (named in its help), and says which it checked.
- The README's "Download a release" gives the exact commands to check a download's checksum and signature, including how to get `freeloop-sign` (`go run` from a checkout at the tag).
- SECURITY.md and REQUIREMENTS.md say what verify-repro compares (the Mac bundle and the Windows exes, two builds on one machine), and no more.
- Tests for the verify paths.

## Notes
- 2026-11-24: Done. `freeloop-sign verify -ignore-missing` (`cmd/freeloop-sign/main.go`, named in the usage, the flag's help and the file's header) checks the signature over the whole `SHA256SUMS` as before, then each listed file the folder holds, skips the absent ones, and prints each file checked and each not there; without the flag an absent file fails and names the flag; with it, a folder holding none of the listed files fails, since then no download was checked (a file saved under another name). The signature check itself is unchanged. Tests: `cmd/freeloop-sign/main_test.go` (whole folder, absent file with and without the flag, a changed file, a list cut to the files present, another version, another key, none present, the usage). README.md's "Download a release" gives the checksum commands, why they do not show who made the list, and the clone, `git checkout v<version>` and `go run ./cmd/freeloop-sign verify … -ignore-missing` commands; since the tool at 0.1.0-beta.1's tag has no `-ignore-missing`, it says to run it from `main` (same key) for that release. SECURITY.md (threat 13) and REQUIREMENTS.md (NFR-SEC-004, SEC-021's acceptance) say what verify-repro compares (the Mac bundle with its SBOM and the two Windows exes, two builds on one machine, not the published files) and no more; DEVELOPING.md names the flag. Not verified against a real published release folder.
