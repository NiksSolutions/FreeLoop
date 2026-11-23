---
title: On Windows the page's own runtime calls are accepted
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23; not yet seen on a Windows machine (issue 055). `netguard.RuntimeCallAllowed` (`internal/netguard/origin.go:47`) accepts a call with an Origin only when its scheme is `wails`. Wails v3.0.0 serves the page from `http://wails.localhost` on Windows, and Chromium puts Origin on every POST, so every engine call would get 403.

## Acceptance
- Read Wails v3.0.0's Windows sources in the module cache to confirm the page's origin there, and write what was found in the notes.
- The app's own origin comes from the platform (NFR-XPLAT-002): on Windows exactly `http://wails.localhost` is accepted, and only when `Sec-Fetch-Site` is absent or `same-origin`; on the other systems nothing changes.
- Test rows for the accepted Windows origin and for look-alikes (`http://wails.localhost.evil.example`, `http://wails.localhost:1`, `https://wails.localhost`) refused. `GOOS=windows CGO_ENABLED=0 go vet ./internal/... ./cmd/...` passes.
- Issue 055 gets a line: check on Windows that the window loads its data.

## Notes
- 2026-11-23: what Wails v3.0.0's sources say (`~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.0.0`): `internal/assetserver/assetserver_windows.go` sets the base URL to `http://wails.localhost` (darwin and linux: `wails://localhost`), and `assetserver.GetStartURL` loads the window from it. `pkg/application/webview_window_windows.go` filters every request (`AddWebResourceRequestedFilterWithError("*", …)`) and serves through the app's asset handler, middleware included, any `http` request whose host starts with `wails.localhost` (`strings.HasPrefix(reqUri.Host, "wails.localhost")`), so `wails.localhost:1` and `wails.localhost.evil.example` reach the app too. A development build (not the `production` tag) with `FRONTEND_DEVSERVER_URL` set adds the dev server's port to the base URL (`build_dev.go`, `GetStartURL`), so there the page's origin is `http://wails.localhost:<port>`. Not seen on a Windows machine.
- 2026-11-23: done. `netguard.RuntimeCallAllowed` (`internal/netguard/origin.go`) takes the platform's `pageOrigin`: a call whose Origin is exactly that, with `Sec-Fetch-Site` absent or `same-origin`, is taken; everything else is as before. `windows.PageOrigin` (`internal/platform/windows/page.go`) is `http://wails.localhost`; `cmd/freeloop/platform.go` has `PageOrigin`, set only in `platform_windows.go`, and `main.go` passes it. Tests: `TestRuntimeCallFromTheWindowsPage` (the accepted origin, same-origin; refused: same-site, cross-site, none, `http://wails.localhost.evil.example`, `http://wails.localhost:1`, `https://wails.localhost`, an http origin with no page origin), `TestPageOriginIsThePlatforms` (`cmd/freeloop/platform_test.go`; it checks the Windows value only when run on Windows). `GOOS=windows CGO_ENABLED=0 go vet ./internal/... ./cmd/...` passes. SECURITY.md (engine calls from a framed page) says so; issue 055 has the line. Left as it is: a development build on Windows (`make dev`), whose origin carries the dev server's port, is still refused.
