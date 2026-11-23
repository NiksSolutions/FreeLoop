---
title: The phone says when the computer is out of reach, and the message box sits on the keyboard
status: done
type: afk
blockedBy: []
prd: sessions-from-the-phone
---

## What
Found in issue 025 on the user's iPhone. After a night in which Tailscale on the Mac was stopped, the home-screen app showed only a white page: it could not load from the computer, and nothing said why. And with the keyboard open, an empty band stood between the message box and the keyboard: the room kept for the home bar, which the keyboard covers.

## Acceptance
- A page load of the app that the network fails gets a page in the phone's language saying the computer cannot be reached, what to check and a link to try again. The service worker still stores nothing, and every other request goes to the network untouched.
- In the open app, a computer out of reach shows the same words with Try again, and the lists claim nothing meanwhile; when the computer answers again, the words go by themselves.
- Typing, the message box sits on the keyboard; not typing, it keeps the home bar's room.
- Worker tests drive the fetch handler; the app check gives the page an iPhone's home-bar room and fails every call to the computer until it lets them through.

## Notes
- 2026-11-23: done. `sw.js` (`offlinePage`, a fetch handler for failed page loads under /phone/), `PhoneApp.tsx` (`unreachable`, the summary stream started at once and taken up again), `phone.css` (`--safe-bottom`, `[data-keyboard]`), `scripts/appcheck/phone-unreachable.js`. The help says to keep Tailscale connected and to start it at login.
