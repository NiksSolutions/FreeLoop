---
title: The AppImage's runtime is pinned
status: done
type: afk
blockedBy: []
---

## What
Found in the first Linux package build after issue 077 (2026-11-24). Every AppImage build downloads the AppImage runtime it puts in front of the squashfs from `https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-<arch>`, through appimagetool inside linuxdeploy's AppImage plugin, with no checksum: both in wails3's own step and in the repack that adds the notices. Everything else the Linux image downloads is pinned by checksum (`build/linux/Dockerfile`), so this is the one unpinned file in a release (SEC-012, NFR-SEC-004), and the runtime is executable code every user runs first.

## Acceptance
- The Dockerfile downloads a pinned type2-runtime release for the image's architecture and checks its SHA-256, as it does for linuxdeploy; the AppImage steps use that file (appimagetool's `--runtime-file`, or the plugin's equivalent) and download nothing.
- The build fails if the runtime file is missing; an AppImage's first bytes equal the pinned runtime's.
- The Linux build in the image (`make linux-package`) passes; DEVELOPING.md says where the pin is and how to move it.

## Notes
- 2026-11-24: done. `build/linux/Dockerfile` downloads type2-runtime `20251108` (`runtime-aarch64`, `runtime-x86_64`) to `/opt/linuxdeploy/runtime`, checked against the SHA-256 GitHub records for the release assets. `build/linux/Taskfile.yml` passes it as `LDAI_RUNTIME_FILE` and `APPIMAGETOOL_RUNTIME_FILE` to wails3's step and to the repack, refuses to start without it (`APPIMAGE_RUNTIME`), and after the build checks that the AppImage's squashfs starts where the runtime ends (`--appimage-offset`) and that its ELF header is the runtime's. `make linux-package` (arm64, Colima): the repack ran appimagetool with `--runtime-file /opt/linuxdeploy/runtime`, nothing was downloaded, and the check passed. amd64 is checked by CI's package-linux on the next tag or dry run.
