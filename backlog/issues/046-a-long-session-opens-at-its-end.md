---
title: A long session opens at its end
status: done
type: afk
blockedBy: [041-the-store-reads-from-where-it-is-asked, 044-the-fold-goes-on-from-where-it-was, 045-a-session-not-on-screen-holds-no-transcript]
prd: before-going-public
---

## What
Only if, with 041 to 045 done, opening the 80,000-event fixture still takes more than a second to show its end, or the 88 MB session still crosses the bridge whole. Then: the window asks for the last few thousand events and for earlier pages as the reader scrolls up, and what the view needs from the whole session (token totals, the mode, git state, the files touched, rewinds) comes from the engine in one small answer, not from folding everything.

## Acceptance
- First, the measurement: the numbers after 045 in this file. If the limit is met, the issue is closed as not needed, with the numbers.
- Opening the long fixture shows its end in under a second, and no call across the bridge carries more than the page size; app check and Go test.
- Scrolling up loads earlier events without the place jumping; app check step, with the checks the flicker fix added still passing.
- The header's totals, the right panel's files and a rewind's greyed range match the fully loaded view; `node --test` comparing both on the long fixture.
- Search and "jump to seq" open the page that holds the seq.
- The phone opens at the end the same way.

## Notes
- The largest change to the window in this plan; see decision 4 in the PRD.

## Measured and built (2026-11-22)
- **Before**, with 041 to 045 in (Apple M4 Pro, lived-in app check, the 80,000-event tools fixture): 3,978 ms from the click to the end drawn, 111 turns, 12,223 items and 105,755 elements in the page. The limit of a second was missed, so this was built. Timing each read in WebKit showed where the time went: the window read the session in 16 pages of 5,000 from seq 0 and redrew the whole growing conversation after each, so each page waited longer for the page to be free (145 ms for the second, 756 ms for the tenth); folding took about a millisecond a page. A closed tool card also built its whole input and output.
- **Built:** `Engine.EventsBefore` (and `EngineService.EventsBefore`, at most `MaxEventsPage`) reads the last events before a seq, or the session's end. The window opens a session with its last 2,000 events (`transcriptFrom`), draws about its last 300 items (`frontend/src/shownRuns.ts`) and more, the place kept, as the reader nears the top or presses "Show earlier messages". 50 ms after the end is drawn it reads what came before a page at a time, back to seq 1, and puts it in front in one go (`withEarlier`), so the view is folded and drawn once more, not per page; until then the cost says "Counting…" and the top of the conversation says the earlier part is being read. A jump to a search hit draws the items around it, with "Show later messages" and "Go to the end" below. A closed tool card draws its line only (`ToolView.tsx`). The phone's session stream takes `tail` and opens with the last 2,000 events, saying earlier ones are on the computer.
- **After** (same machine and check): 244 ms from the click to the end drawn, 300 items in the page; what came before read and folded in 1.8 s, with the first item drawn the same and the view still at the end; scrolling up draws 300 more with what was first 41 px from where it was (drawn items settling to their heights); a search hit at seq 49,678 of 80,000 drawn and in view in about a second, 600 items in the page.
- **Not as the issue said:** the totals, the right panel and the rewound range do not come from a small answer of the engine. Folding the whole session takes milliseconds once its events are in, and a second fold of the same events in Go would be one more thing to keep equal; so the window reads the rest in pages after drawing the end and folds it itself. The whole session still crosses the bridge, but after the end is on screen and never more than a page per call.
- **Tests:** `TestEventsBeforeReadsBackAPageAtATime` (pages, and no read above `MaxEventsPage`), `TestPhoneStreamOpensAtTheEnd`, `shownRuns.test.ts`, and in `transcript.test.ts` the long fixture opened at its end and given what came before equals the whole (totals, the panel's files and a rewind reaching back past where the end began). The lived-in app check holds the time to a second (only printed under CI or `APPCHECK_SLOW`), and checks scrolling up, the search hit far back and Go to the end. The time is the app's own (`probe.drawn`, set in a layout effect), since the check's timers wait a second each while its window is behind others; macOS does the same to `requestAnimationFrame`, which is why the read of what came before starts on a timer.

