---
title: Pictures from the phone
status: done
type: afk
blockedBy: []
prd: sessions-from-the-phone
---

## What
Found in issue 025 on a real iPhone: a screenshot or photo cannot be pasted or attached to a message from the phone. The engine takes pictures in a message (SES-015) from the computer's window; the phone's message path carried text only.

## Acceptance
- The phone's message box has a picture button that opens the phone's photo picker or camera, and a picture pasted into the box is attached too.
- Attached pictures show as thumbnails that can be removed before sending; they are scaled down as on the computer (`imageprep.ts`), at most 10 to a message.
- A message may be pictures alone. The engine checks each picture's bytes and stores it as one of the session's attachments, as it does for the computer's; the message says it came from the phone.
- The phone shows a message's pictures, fetched with its key; a picture is only ever the user's own attachment, never something a model wrote.
- The phone server takes a message with pictures up to what 10 scaled pictures need, and every other call stays within 64 KB.
- Tested in `internal/remote`, and the app check attaches a picture on the phone and sees it in the conversation.

## Notes
- 2026-11-22: done. `TestPhoneSendsPictures` and `TestPhoneBodyLimits` in `internal/remote/pictures_test.go`; the phone pass of `make check-app` attaches, pastes, removes and sends a picture and sees it, read back with the phone's key, beside "From the phone".
