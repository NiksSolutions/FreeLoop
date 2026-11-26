---
title: The sandbox keeps instruction files in every folder (OQ-060)
status: done
type: afk
blockedBy: []
---

## What
OQ-060 in `docs/OPEN_QUESTIONS.md`, decided by the user on 2026-11-26 for (b): a sandbox keeps the names something reads from a subfolder (`AGENTS.md`, `CLAUDE.md`, `.envrc`) at any depth, tried on a real npm project and a real Linux project before it ships.

## Acceptance
- A sandboxed command cannot create or change `src/AGENTS.md`, `src/deep/CLAUDE.md` or `sub/.envrc`, in any spelling the file system opens as the same.
- `npm ci`, `rm -rf node_modules` and `npm uninstall` still work on a real lockfile whose packages ship such files.
- The rest of `permission.ActedOnLater` stays protected at the top only. OQ-060 is closed with the outcome and date.

## Notes
- 2026-11-26: Done. `permission.ReadFromSubfolders` (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.envrc`; Gemini CLI reads `GEMINI.md` from subfolders too) and `environment.InstalledDirs` (`node_modules`, `bower_components`, `.venv`, `venv`, `site-packages`, `vendor`, `Pods`) become `Confinement.ProtectedNames` and `Confinement.Installed` in `internal/engine/sandbox.go`. Seatbelt (`seatbelt.go`) denies every write but `file-write-unlink` on `^root/(.+/)?(names)$`, `require-not` inside an installed folder; each name is spelled out with `unicode.SimpleFold` (`anySpelling`), since APFS opens `AGENTſ.md` as `AGENTS.md`. Found: Seatbelt compares paths without case, so `Node_Modules` counts as installed, and the agent's check (`installed` in `internal/harness/builtin/builtin.go`) and bubblewrap's compare in any case too. bubblewrap (`matching` in `bubblewrap.go`) adds each existing match under Write outside installed code to the read-only mounts. The built-in agent skips nested `AGENTS.md` inside installed code (`discover`). Found while measuring: of 809 packages installed for a Next.js-and-AI-SDK project, `next` ships `AGENTS.md` and `effect` ships `AGENTS.md` and `CLAUDE.md`, so (b) without the installed folders would have broken `npm install next`. Tried on a Mac in the engine's sandbox with that lockfile: `npm ci` (6 s), `rm -rf node_modules`, `npm ci` again and `npm uninstall next` ran, `src/AGENTS.md` refused. On Linux `TestProtectedNamesAreFound` passes in the build image; the tests that run bubblewrap need unprivileged user namespaces, which Colima's Ubuntu 24.04 restricts through AppArmor (`make linux-test-sandbox`), so they and the npm try on Linux were left for issue 055 by the user's choice.
