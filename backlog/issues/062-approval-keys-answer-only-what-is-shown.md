---
title: An approval key answers only the request shown, and never a plan
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (GUI-017, GUI-043). The conversation's key listener (`frontend/src/ToolView.tsx:60-79`) leaves out forms but not plan cards: Cmd+Enter while typing feedback on a plan approves it, `y` sends the first allow-once option whatever the plan's select shows, and with an external agent `a` can choose auto mode. The Attention view's listener (`frontend/src/Attention.tsx:167-178`) has no guard at all: a held `y` approves one request after another across sessions, and a key lands on whichever request became current a moment before.

## Acceptance
- A waiting plan is answered only through its own controls: no approval key decides it, in the conversation or in Attention. In Attention a plan request shows no ambiguous buttons: it offers to open the session.
- Attention's keys ignore key repeat, a prevented or composing event, and any key within the grace period after the current request changes (the same `APPROVAL_GRACE_MS`), through one guard shared with the conversation card.
- In the conversation, the grace period starts when a card becomes the one the keys act on, and typing a note does not restart it.
- No approval key acts while a modal dialog covers the card.
- Unit tests for the shared guard; `scripts/appcheck` steps: Cmd+Enter in a plan's feedback box does not approve the plan; a second `y` right after the first in Attention does not answer the next request.
- `make test`, `make lint` and `make check-app` pass.

## Notes
- 2026-11-23: done. Each defect confirmed by reading at cd40672, and the conversation's and Attention's by the app check before the fix ("⌘↩ in a plan's feedback box does not approve it", "Y does not approve a plan", "a second Y right after the first does not answer the next request" and "a held Y's repeat answers nothing" failed). One guard, `approvalKey` in `frontend/src/shortcuts.ts`, now decides for both views: a key's repeats, a prevented or composing event and any key while an `[aria-modal="true"]` dialog covers the card (`coveredByDialog`) answer nothing, and a key within `APPROVAL_GRACE_MS` of the card or request becoming the one the keys answer is taken but answers nothing. `ApprovalTarget` remembers which that is and since when, so a redraw (a note typed, a scope chosen) no longer restarts the grace; `ToolView.tsx` updates it when a waiting card appears or is answered and `SessionView.tsx` when its pane becomes the active one, and the listener's effect no longer depends on the note and scope. A plan (`isPlan` in `planView.ts`, by kind and input as `planText`, which now shares it) gets no key listener in either view; in `Attention.tsx` its request shows a line saying it is answered in its session and a "Review the plan" button opening it, no note or answer buttons. Words in en, de and nl (`attention.planInSession`, `attention.reviewPlan`); the keyboard help (three languages), GUI-006's as-built note in REQUIREMENTS.md and a section in SECURITY.md say so. Tests: `shortcuts.test.ts` (the guard; the grace from becoming the target), `planView.test.ts` (a request is a plan by kind and input); app check (`scripts/appcheck/scenario.js`): in the plan step, ⌘↩ in the feedback box and Y leave the plan waiting, and in Needs attention the plan offers its session, Y leaves it, and "Review the plan" opens it still waiting; a new step: in Needs attention a second Y right after the first and a held Y's repeat leave the next request waiting, and Y answers it a moment later. Not done: J and K in Attention keep moving on a held key, as they did; a second key on the conversation's card before its answer has come back still asks the engine again, which says the request is unknown.
