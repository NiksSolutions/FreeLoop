---
title: Pair a phone by scanning a QR code
status: done
type: human
blockedBy: []
prd: sessions-from-the-phone
---

## What
Settings → Phone shows the pairing link as a QR code beside the code, so the phone's camera opens it. The engine renders it as a PNG with `rsc.io/qr` (BSD-3, about 1,000 lines, no further dependencies), passed to the page as a data URL; the typed code stays.

## Acceptance
- The QR decodes to the shown link; Go test decoding the PNG.
- `make check-app` finds the QR image with an accessible name while a pairing window is open.

## Notes
- **Waits for the user to agree to `rsc.io/qr`** (CLAUDE.md: ask before adding a dependency); asked on 2026-11-11, no answer yet. Once they agree, an agent can do it; if they decline, close it as not done.
- 2026-11-22: the user agreed to `rsc.io/qr` (v0.2.0). Done: `remote.PairingQR` draws the link at level M as a PNG with its quiet zone, `EngineService.PhonePairingQR` hands it to the Phone page, which shows it above the code with the alt text "QR code of the address below". `TestPairingQRHoldsTheLink` reads the modules back from the PNG, finds the level and mask from the format bits, checks each block's Reed-Solomon bytes and gets the link; macOS's own detector (Core Image) read the same link from a generated PNG once by hand. `make check-app` waits for the decoded image and its accessible name in the pairing window.
