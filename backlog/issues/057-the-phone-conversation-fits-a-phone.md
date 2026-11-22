---
title: The phone's conversation fits a phone and reads like the desktop's
status: done
type: afk
blockedBy: []
prd: sessions-from-the-phone
---

## What
Found in issue 025 on a real iPhone: chats do not scale well and the layout is not aligned. The phone app styled none of what the Markdown renderer draws, so a table broke its words letter by letter ("Jo b", "Re su lt"), a code block had a Copy button as tall as a list row and no frame, inline code was unmarked, and paragraphs kept the browser's margins, leaving uneven space in every agent message. A tool's ✓ stood on a line of its own above its title. The message box and a full-width Send button took a quarter of the screen. Worst, the whole page was wider than the screen and shifted sideways (the user's screenshot): a token with no break in it, a hash or a path in a tool's command or a notice, widened the conversation's grid or spilled out of its line. And it looked like another app: an accent bubble for the user, a grey one for every agent reply, none of the desktop's colours.

## Acceptance
- At an iPhone's width (390 px) nothing is wider than the screen and no text spills out of its box, hashes and paths included; a wide table or a long code line scrolls inside its own box, and no table cell breaks a word.
- A tool call is one line, its mark beside its title; tapping it shows the whole title.
- Agent messages are text as on the desktop, user messages a framed bubble on the right; the colours are the desktop's, light and dark.
- Paragraphs, lists, headings, inline code, quotes, tables and code blocks are spaced and marked as on the desktop.
- The message box is one row with its buttons beside it, and grows with what is typed.
- The app check opens the phone app at 390 px on a session made of such content and checks the above.

## Notes
- `scripts/appcheck/phonesession.ts` writes the session; the driver takes `APPCHECK_WIDTH` and `APPCHECK_INIT` (a script run before the page, which puts a paired phone's key in its storage).
- 2026-11-22: done. `phone.css` takes the desktop's colours and Markdown rules; a tool call is a `details` line; the message box is one row that grows. The phone pass of `make check-app` checks each point above at 390 px.
