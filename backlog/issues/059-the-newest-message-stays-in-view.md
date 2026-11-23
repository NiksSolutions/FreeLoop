---
title: The phone keeps the newest message in view when the keyboard opens
status: done
type: afk
blockedBy: []
prd: sessions-from-the-phone
---

## What
Found in issue 025 on a real iPhone, in the user's screenshot: with the keyboard open, the newest message slipped under the message box. The keyboard shrinks what the page can show, and the conversation stayed where it was instead of keeping its end in view; the message box growing did the same. And any new event pulled the reader to the end, even while they had scrolled up to read.

## Acceptance
- At the end of a conversation, the newest message stays just above the message box when the keyboard opens or closes and when the message box grows.
- Scrolled up, the reader stays where they are, whether the keyboard opens or events arrive.
- Sending a message goes to the end.
- The app check imitates the keyboard by shrinking the window and checks the above.

## Notes
- 2026-11-22: done. `SessionScreen` in `PhoneApp.tsx` notes whether the reader is at the end from scrolls made at a settled size (the browser scrolls by itself while what the page shows changes size) and keeps the end in view on a resize of the window or the visual viewport, and on a resize of the conversation or the message box. The driver takes a `resize` message for the check.
- 2026-11-23: the user confirmed it on the iPhone: with the keyboard open, the newest message stays above the message box.
