---
title: The AppImage's runtime is pinned
status: open
type: afk
blockedBy: []
---

## What
Found in the first Linux package build after issue 077 (2026-11-24). Every AppImage build downloads the AppImage runtime it puts in front of the squashfs from `https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-<arch>`, through appimagetool inside linuxdeploy's AppImage plugin, with no checksum: both in wails3's own step and in the repack that adds the notices. Everything else the Linux image downloads is pinned by checksum (`build/linux/Dockerfile`), so this is the one unpinned file in a release (SEC-012, NFR-SEC-004), and the runtime is executable code every user runs first.

## Acceptance
- The Dockerfile downloads a pinned type2-runtime release for the image's architecture and checks its SHA-256, as it does for linuxdeploy; the AppImage steps use that file (appimagetool's `--runtime-file`, or the plugin's equivalent) and download nothing.
- The build fails if the runtime file is missing; an AppImage's first bytes equal the pinned runtime's.
- The Linux build in the image (`make linux-package`) passes; DEVELOPING.md says where the pin is and how to move it.
