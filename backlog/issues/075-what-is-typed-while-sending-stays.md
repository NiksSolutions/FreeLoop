---
title: What is typed while a message is being accepted stays
status: done
type: afk
blockedBy: []
---

## What
Found in the review of 2026-11-23 (GUI-003). `send()` in `frontend/src/SessionView.tsx:878-892` (and `runStage` `:806-820`, `runPrompt` `:496-507`) awaits the engine, which for an external agent's first message can take long (it starts or downloads the agent), then clears the draft, attachments and pictures unconditionally. What the user typed or attached meanwhile is erased, from the stored draft too.

Also from the review, small and in the same files' neighbourhood:
- `AFKDialog.tsx`, `AFKRunView.tsx`: `aria-modal` dialogs that take no focus and do not hold or return it.
- `AttachedImages.tsx:33-39`: one failed attachment read shows "missing" for the rest of the run.
- `SessionView.tsx:1328`: the rename field has no Escape; blur saves a half-typed title.
- `DiffPanel.tsx:305-334`: a review comment being written is anchored by row index and can land on another line when the diff refreshes; replies have no newest-wins guard.

## Acceptance
- After a send resolves, only what was sent is cleared: text typed since and attachments added since stay, in the field and in the stored draft. An app-check step with a slow first send, or a unit test of the extracted logic if the app check cannot slow it.
- The AFK dialogs focus their first control, keep Tab inside, and return focus on close.
- A failed attachment read is retried on the next mount. Escape cancels a rename. A comment keeps its line across a refresh, and a stale diff reply does not replace a newer one.
- `make check-app` passes.

## Notes
- 2026-11-24: once the engine takes a message, a stage or an MCP prompt, only what it took leaves the composer: `forgetSent`, `unsentText` and `unsentOf` in `frontend/src/drafts.ts` clear the sent text (keeping what was typed after it, or the whole text when the sent part was changed) and the attachments, files and pictures sent, in memory and in the stored draft; `SessionView.tsx` uses them in `send`, `runStage` and `runPrompt`. Unit-tested in `drafts.test.ts`; the app check cannot slow a first send (the demo agent answers at once), so it is the unit test of the extracted logic. Also: the AFK dialogs hold the focus (`useModalFocus.ts`, `tabWrap` in `a11y.ts`; the app check sees the first control focused and Tab go round); a failed picture read is not kept, so the next view reads it again (`readPicture` in `images.ts`, `AttachedImages.tsx`); Escape cancels the header's rename (app check); a comment being written keeps its line across a diff refresh (anchored with `anchor`/`locate`), and the Diff tab's lists and diffs use the newest reply only (`newest()` in `DiffPanel.tsx`). `make check-app` passes. SES-023 in REQUIREMENTS.md says so.
