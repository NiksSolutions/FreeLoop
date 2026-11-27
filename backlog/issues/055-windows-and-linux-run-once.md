---
title: Windows and Linux are run once on a real machine
status: open
type: human
blockedBy: []
prd: before-going-public
---

## What
OQ-045 says macOS, Windows and Linux work essentially the same before the repository goes public. The Windows code was cross-compiled and vetted on a Mac and has never run on Windows; the window has only ever been driven in macOS WebKit. Someone installs the package on a Windows 11 machine and on a Linux desktop and runs the demo and one real session on each.

## Acceptance
- On each: install, first run, the demo to its end, a session on a real agent with one approval, a terminal tab, quit and reopen with the session there.
- On Windows the window loads its data: the sidebar lists projects and sessions and Settings opens, so the engine takes the page's calls from `http://wails.localhost` (issue 066); the log has no "refused a runtime call".
- The conversation follows its end while streaming and holds its place when scrolled up (Chromium anchors scrolling itself, which `drawOnce` was not written for); a PDF opens; the message box grows with its text.
- On Windows, git as the app runs it (OQ-046), with Git for Windows installed with its defaults (`core.autocrlf=true`, `credential.helper=manager`): in a repository checked out with CRLF line endings the diff panel and the session's change count show no changed files, also after `touch`ing some; a repository with `git config core.autocrlf false` keeps its LF files LF through a discard in the panel and a session worktree; Create pull request pushes over https and signs in through Git Credential Manager's own window, with no certificate error (OQ-061). With another helper in the system config (`git config --system credential.helper wincred`, as an administrator) and the app restarted, its push signs in as a terminal's `git push` does.
- On Linux, voice input (OQ-048) with a real microphone, from the `.deb` on Ubuntu 22.04: the button shows in the message box, a dictation records (the level moves) and comes back as text, in the main window and in a session's own window; then the same from the AppImage. A page from a local server in the preview that calls `navigator.mediaDevices.getUserMedia({audio: true})` is refused. On a system on PipeWire (Ubuntu 24.04 or Debian 12) the `.deb` records as well, while the AppImage is expected to find no microphone (OQ-048, its GStreamer).
- On Linux, the sandbox's own tests on a machine that allows unprivileged user namespaces (`FREELOOP_SANDBOX_TEST=1 go test ./internal/platform/linux/`, or `make linux-test-sandbox` once Colima's VM allows them), `TestTheSandboxKeepsInstructionsBelowTheTop` among them (OQ-060, issue 090); then, in a "Local, sandboxed" session on a project with `next` installed, `npm ci` and `rm -rf node_modules` work and `echo x >> src/AGENTS.md` is refused.
- On Linux, a "Local, sandboxed" session on a machine with a large Go module cache (`~/go/pkg/mod`) or Rust toolchain runs its commands, and a project of some tens of thousands of files does not slow each one noticeably: before 2026-11-27 the sandbox counted the toolchains' files against its 200,000-entry walk and could refuse every command (found by reading, not yet seen on a desktop).
- What fails becomes an issue; what blocks a first session is fixed before 056.
- If no machine is at hand: the README says in its first screen that Windows and Linux are untested, and OQ-045 gets a note that the repository went public that way.

## Notes
- A virtual machine counts. Needs a person.
- 2026-11-22: no machine at hand yet, so the fallback is done: the README's first screen says the Windows and Linux builds have not been run on a desktop, and OQ-045 notes that the repository went public that way. The issue stays open for the first run on a real machine (or a virtual one).
- 2026-11-25: OQ-046 is closed and built (issue 087); the line on git on Windows under Acceptance is what to check there, since it was tested only on a Mac with stand-in system configs.
- 2026-11-25: OQ-048 is closed and built (issue 088), tried only in the Linux build image under Xvfb with WebKit's mock microphone; the line on voice input on Linux under Acceptance is what to check on a desktop.
