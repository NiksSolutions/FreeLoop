---
title: Settings are saved by one writer, as changes
status: open
type: afk
blockedBy: []
---

## What
The Settings page saves the whole config by itself 250 ms after a change (SET-007), and the main window saves the whole config too: the text size keys 400 ms after a press, the fonts and the style chosen 300 ms after a change (NFR-A11Y-002, GUI-025, GUI-053). Each sends what it last saw, and each puts what the engine answers into the window's config. Two saves under way at once can end in either order, so an older one can win: on disk, and in the window, whose shortcuts and text size follow its config. CI's macOS runner, where a save can take longer than the check's steps, showed it three times while beta.2 was made: a Reset of a recorded shortcut came back as the shortcut (tag run of 69504cb), a style chosen from the palette was put back by a late save (dry run of 202c594), and the window kept a recorded shortcut after the page had saved the Reset (dry run of e2e8c62). A person is rarely that fast, but a slow disk makes the window wider.

Fixed for beta.2, in `frontend/src/Settings.tsx`: a save that ends after a further change no longer replaces the form (bed7cbe), and the form is looked at again when a save ends (e2e8c62). The check-app step now waits for each Saved, as a person would; it does not prove the race gone.

## Acceptance
- One place sends config changes to the engine, one at a time and in the order made, and only the fields changed (or the engine merges a change into what it has), so a change never carries another writer's older values.
- What the engine answers reaches the window only when no later change is waiting, so the window never shows, or acts on, a state older than one it already had.
- check-app's shortcut step without its waits (record, Reset 0.3 s later, leave 0.1 s after) passes with every save slowed to 1.5 s, and so do the language, style and text size steps.

## Notes
- Tried on 2026-11-26 and left out of beta.2, in `Settings.tsx` alone: saves queued one after another, a change compared with the save under way rather than with the config from before it, and a superseded answer dropped. It passed check-app at normal speed; with every answer 1.5 s late, queued saves added up past "bypass is in config.yaml"'s wait, and it does not stop the main window's own saves carrying an older text size or style.
