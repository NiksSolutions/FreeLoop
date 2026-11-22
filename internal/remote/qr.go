package remote

import (
	"encoding/base64"
	"errors"

	"rsc.io/qr"
)

// maxQRText bounds what PairingQR draws: a pairing link is under 100 bytes.
const maxQRText = 512

// PairingQR draws a pairing link as a QR code (GUI-052): a PNG, as a data URL the
// Phone page shows beside the code, so the phone's camera opens the link. Level M
// survives a glare or a smudge on the screen; the PNG carries the four-module
// quiet zone a camera needs.
func PairingQR(link string) (string, error) {
	if link == "" || len(link) > maxQRText {
		return "", errors.New("a pairing link is drawn only up to 512 bytes")
	}
	code, err := qr.Encode(link, qr.M)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()), nil
}
