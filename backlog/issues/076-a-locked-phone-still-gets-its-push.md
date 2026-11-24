---
title: A locked phone still gets its push
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (GUI-052; real phones not yet tried for push, issue 030). `Notify` (`internal/remote/notify.go:40`) sends a phone no push while it has an open stream. The phone app never closes its streams when hidden, the server's pings keep the socket alive, and stream writes have no deadline, so a locked phone with the app open counts as looking and gets nothing.

Also from the review, same package:
- `stream.go:165-177, 80-86`: events the bus dropped are noticed only when a later event for that session arrives; a dropped `turn_ended` leaves the phone showing a working session.
- `push.go:32-37, 52`: `vapidKey` holds `s.mu` across keyring calls that can wait on a prompt, blocking every phone request and `Status()`.
- `server.go:238-245, 574-588`: a device is saved when the user allows, before the phone collected its key; a lost connection leaves a paired phone nobody holds. `pairAnswer` reads `s.pairing` without the expiry check.
- `cmd/freeloop/main.go:298-303`: the notice loop calls `desk.notice` synchronously, which can wait 180 s on the system's permission prompt while later notices queue and drop.

## Acceptance
- The phone app closes its streams when the page is hidden and reopens them when shown; only open stream handlers count as looking; each stream write has a deadline.
- The keep-alive tick also catches up, so a dropped last event arrives within one tick.
- The push key is read outside `s.mu`. A device exists only once its key was handed out, or an uncollected one is removed when the window closes; `pairAnswer` honours the expiry. The desktop notice does not hold up the loop.
- Go tests for each; phone unit tests for the visibility handling; `make check-app` passes.

## Notes
- 2026-11-24: the phone app holds its streams open only while its page is shown (`whileShown` in `frontend/src/phone/phone.ts`, used by `follow` in `phone/api.ts`); closing them so does not call `onLost`, so the out-of-reach notice is unaffected, and shown again a session's stream goes on from its last event. Server side (`internal/remote`): only open stream handlers count as looking (`Server.looking`, `openStream` in `stream.go`; `Notify` reads it); each write and flush to a stream has a 10 s deadline; the keep-alive tick catches up a session's events and resends a changed summary, so a dropped last event arrives within a tick; the push key is read from the keyring outside `s.mu` (`vapidMu` in `push.go`), made once; a phone allowed on the computer becomes a device only when `pairAnswer` hands out its key, which honours the window's expiry (`server.go`). The existing tests pinned allow, then collect, then a second collect refused; that order is unchanged. `cmd/freeloop/main.go`: `relayNotices` gives the desktop its own queue, so its permission prompt holds up neither the loop nor the phone. Tests: `TestOnlyAnOpenStreamIsLooking`, `TestAStalledStreamEnds`, `TestADroppedLastEventArrivesWithinATick`, `TestThePushKeyIsReadOutsideTheLock`, `TestAPhoneIsPairedOnlyOnceItTookItsKey`, `TestADesktopNoticeHoldsUpNothing`, the phone's visibility tests in `phone.test.ts`, and a phone app-check step that hides and shows the page. `make check-app` passes. Not tried on a real phone; issue 030 now says what to check there. Route authentication is unchanged.
