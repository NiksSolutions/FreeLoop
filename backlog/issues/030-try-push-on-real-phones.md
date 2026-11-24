---
title: Try push on an iPhone and an Android phone
status: open
type: human
blockedBy: [025-try-the-phone-on-a-real-phone, 029-sessions-that-need-you-reach-the-phone]
prd: sessions-from-the-phone
---

## What
With the app on each phone's home screen and notifications turned on, close Freeloop's window, lock the phones, and let a session ask for a permission and then finish.

## Acceptance
- Each phone shows "A session needs you" within seconds, and the tap opens the session.
- Allowing it there lets the session go on; its finishing arrives as a second notification.
- Nothing about the session's content shows in the notification before the app opens.
- With local-only mode on, no notification comes.

## Notes
- Needs a person and both phones; anything that fails becomes an issue.
- 2026-11-24 (issue 076): the phone app now closes its streams when its page is hidden. Check on each phone that the push comes with the app left open in front and the phone then locked, and with another app brought to the front; and that unlocking, or coming back to the app, opens the streams again without the "Can't reach your computer" notice. Not tried on a real phone: whether the closing reaches the computer before the phone suspends the page is what only a real one shows.
