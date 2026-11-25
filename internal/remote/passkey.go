package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
)

// Passkeys (OQ-052, SEC-020). A phone's key alone lets it read its sessions and stop a
// turn. To answer a request, or to send a session a message, which can make an agent act
// within its mode, the phone must also have shown its passkey, with Face ID, Touch ID or
// its code, within the last 18 hours, measured here from its last good check. The
// passkey is made at pairing for the name the phone reaches this computer by (the
// relying party); a phone paired before has none and makes one before it answers. Only
// the public key is kept, as the browser hands it over (getPublicKey(), SPKI), so both
// ceremonies are checked with the standard library and no CBOR is read: attestation is
// "none", so the key in the authenticator's data proves nothing more.

const (
	// checkLife is how long a passkey check lets a phone answer and send messages.
	checkLife = 18 * time.Hour
	// challengeLife is how long a challenge may wait for its answer; it is used once.
	challengeLife = 2 * time.Minute
	// es256 is the one algorithm taken: ECDSA on P-256 with SHA-256, COSE -7, what
	// every platform authenticator makes.
	es256 = -7
)

// The authenticator data's flags (WebAuthn §6.1).
const (
	flagUP = 0x01 // the user was present
	flagUV = 0x04 // the user was verified: Face ID, Touch ID, the phone's code
	flagAT = 0x40 // a credential follows: at registration
)

// What a phone may be asked for, in the "passkey" field of a refusal: a passkey to
// make, a check because the last is too old, or pairing again, as its passkey was made
// for another name than the one set now.
const (
	needMissing   = "missing"
	needStale     = "stale"
	needPairAgain = "pair-again"
)

// challenge is one a phone was given and has not used: one per phone, the newest.
type challenge struct {
	value   []byte
	kind    string // "webauthn.create" or "webauthn.get"
	expires time.Time
}

// hostName is the name in remote.host, as a request's Host and a passkey's relying
// party compare it: lower case, with no port and no final dot.
func hostName(host string) string {
	want := strings.ToLower(strings.TrimSuffix(host, "."))
	if h, _, err := net.SplitHostPort(want); err == nil {
		want = h
	}
	return strings.TrimSuffix(want, ".")
}

// relyingParty is the name passkeys are made for and the page that may use them: the
// tailnet name set in Settings, served by the command the Phone page shows on
// engine.PhoneHTTPSPort. No name, no passkeys.
func (s *Server) relyingParty() (rpID, origin string) {
	s.mu.Lock()
	host := s.state.Host
	s.mu.Unlock()
	rpID = hostName(host)
	if rpID == "" {
		return "", ""
	}
	return rpID, "https://" + net.JoinHostPort(rpID, strconv.Itoa(engine.PhoneHTTPSPort))
}

// need says what a phone must do before it may answer or send a message: "" when its
// passkey was checked less than checkLife ago. A clock put back does not stretch a check.
func (s *Server) need(d config.Device, rpID string) string {
	switch {
	case d.Passkey == nil:
		return needMissing
	case d.Passkey.RPID != rpID:
		return needPairAgain
	}
	age := s.now().Sub(d.Passkey.Checked)
	if age < 0 || age >= checkLife {
		return needStale
	}
	return ""
}

// checked is authed, and also needs a passkey check less than checkLife old.
func (s *Server) checked(h handler) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, d config.Device) {
		rpID, _ := s.relyingParty()
		switch s.need(d, rpID) {
		case "":
			h(w, r, d)
		case needMissing:
			writeJSON(w, http.StatusForbidden, map[string]string{"passkey": needMissing, "error": "this phone has no passkey yet; add one in the phone app"})
		case needPairAgain:
			writeJSON(w, http.StatusForbidden, map[string]string{"passkey": needPairAgain, "error": "this phone's passkey was made for another name of the computer; remove the phone in Freeloop's Settings, Phone, and pair it again"})
		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"passkey": needStale, "error": "show this phone's passkey first: it is asked for again after 18 hours"})
		}
	})
}

// passkeyStatus tells the phone app whether it has a passkey and how long its last
// check lasts.
func (s *Server) passkeyStatus(w http.ResponseWriter, r *http.Request, d config.Device) {
	rpID, _ := s.relyingParty()
	out := map[string]any{"need": s.need(d, rpID)}
	if d.Passkey != nil {
		out["until"] = d.Passkey.Checked.Add(checkLife)
	}
	writeJSON(w, http.StatusOK, out)
}

// passkeyChallenge gives the phone a challenge: to make its passkey when it has none,
// otherwise to show it. A new one replaces the phone's last.
func (s *Server) passkeyChallenge(w http.ResponseWriter, r *http.Request, d config.Device) {
	rpID, _ := s.relyingParty()
	if rpID == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "set the name the phone reaches this computer by in Freeloop's Settings, Phone, first"})
		return
	}
	if s.need(d, rpID) == needPairAgain {
		writeJSON(w, http.StatusConflict, map[string]string{"passkey": needPairAgain, "error": "this phone's passkey was made for another name of the computer; remove the phone in Freeloop's Settings, Phone, and pair it again"})
		return
	}
	value := make([]byte, 32)
	rand.Read(value)
	kind := "webauthn.get"
	if d.Passkey == nil {
		kind = "webauthn.create"
	}
	s.mu.Lock()
	if s.challenges == nil {
		s.challenges = map[string]challenge{}
	}
	s.challenges[d.ID] = challenge{value: value, kind: kind, expires: s.now().Add(challengeLife)}
	s.mu.Unlock()
	o := map[string]any{"challenge": rawKey.EncodeToString(value), "rpId": rpID, "timeout": challengeLife.Milliseconds()}
	if d.Passkey == nil {
		// The user handle is the phone's ID, and the passkey is named after the phone.
		o["userId"], o["userName"] = rawKey.EncodeToString([]byte(d.ID)), d.Name
		writeJSON(w, http.StatusOK, map[string]any{"create": o})
		return
	}
	o["credentialId"] = d.Passkey.ID
	writeJSON(w, http.StatusOK, map[string]any{"get": o})
}

// takeChallenge is the phone's challenge of kind, gone once taken, whatever the answer
// turns out to be: a challenge is good for one try, and for challengeLife.
func (s *Server) takeChallenge(id, kind string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.challenges[id]
	delete(s.challenges, id)
	switch {
	case !ok:
		return nil, errors.New("no challenge is open for this phone; ask for one first")
	case c.kind != kind:
		return nil, errors.New("the challenge was for something else")
	case !s.now().Before(c.expires):
		return nil, errors.New("the challenge is too old; try again")
	}
	return c.value, nil
}

// passkeyBody is what the phone app sends of the browser's answer, each part base64url.
type passkeyBody struct {
	ID                string `json:"id"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	// At registration: the key as getPublicKey() gives it, and its algorithm.
	PublicKey string `json:"publicKey"`
	Algorithm int    `json:"algorithm"`
	// At a check: the signature over the authenticator data and the client data's hash.
	Signature string `json:"signature"`
}

// decode reads the named parts of the body.
func decode(parts ...string) ([][]byte, error) {
	out := make([][]byte, len(parts))
	for i, p := range parts {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(p, "="))
		if err != nil || len(b) == 0 {
			return nil, errors.New("the passkey's answer is damaged")
		}
		out[i] = b
	}
	return out, nil
}

// ceremony checks what the browser and the authenticator say of an answer to issued
// (WebAuthn §7.1, §7.2): the client data's type, challenge and page, not in a frame of
// another; the authenticator data's relying party, and that the user was present and
// verified. It returns the authenticator's counter.
func ceremony(clientDataJSON, authData []byte, kind string, issued []byte, rpID, origin string) (uint32, error) {
	var c struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin bool   `json:"crossOrigin"`
	}
	if err := json.Unmarshal(clientDataJSON, &c); err != nil {
		return 0, errors.New("the browser's client data is damaged")
	}
	if c.Type != kind {
		return 0, fmt.Errorf("the browser's answer is a %q, not a %q", c.Type, kind)
	}
	got, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(c.Challenge, "="))
	if err != nil || subtle.ConstantTimeCompare(got, issued) != 1 {
		return 0, errors.New("the answer is not to the challenge this computer gave")
	}
	if c.Origin != origin {
		return 0, fmt.Errorf("the answer was made on %q, not on %q", c.Origin, origin)
	}
	if c.CrossOrigin {
		return 0, errors.New("the answer was made in a frame")
	}
	if len(authData) < 37 {
		return 0, errors.New("the authenticator's data is too short")
	}
	rpHash := sha256.Sum256([]byte(rpID))
	if subtle.ConstantTimeCompare(authData[:32], rpHash[:]) != 1 {
		return 0, fmt.Errorf("the passkey is not for %s", rpID)
	}
	if authData[32]&flagUP == 0 {
		return 0, errors.New("the authenticator says no one was there")
	}
	if authData[32]&flagUV == 0 {
		return 0, errors.New("the phone did not verify its user (Face ID, Touch ID or its code)")
	}
	return binary.BigEndian.Uint32(authData[33:37]), nil
}

// p256Key reads an SPKI public key, which must be ECDSA on P-256.
func p256Key(spki []byte) (*ecdsa.PublicKey, error) {
	k, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return nil, errors.New("the passkey's public key cannot be read")
	}
	pub, ok := k.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, errors.New("the passkey's key is not ECDSA on P-256")
	}
	return pub, nil
}

// passkeyRegister keeps the phone's new passkey. A phone with one cannot make another
// with its key alone, which would let whoever holds the key replace it: a new one comes
// with pairing again. Making it, the user verified, counts as a check.
func (s *Server) passkeyRegister(w http.ResponseWriter, r *http.Request, d config.Device) {
	var body passkeyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	if d.Passkey != nil {
		s.takeChallenge(d.ID, "")
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this phone has a passkey; for a new one, remove the phone in Freeloop's Settings, Phone, and pair it again"})
		return
	}
	value, err := s.takeChallenge(d.ID, "webauthn.create")
	if err != nil {
		failed(w, http.StatusForbidden, err)
		return
	}
	rpID, origin := s.relyingParty()
	parts, err := decode(body.ID, body.ClientDataJSON, body.AuthenticatorData, body.PublicKey)
	if err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	id, clientData, authData, spki := parts[0], parts[1], parts[2], parts[3]
	count, err := ceremony(clientData, authData, "webauthn.create", value, rpID, origin)
	if err != nil {
		failed(w, http.StatusForbidden, err)
		return
	}
	// The credential the authenticator made: AAGUID (16 bytes), the ID's length, the ID.
	if authData[32]&flagAT == 0 || len(authData) < 55 {
		failed(w, http.StatusBadRequest, errors.New("the authenticator's data holds no passkey"))
		return
	}
	n := int(binary.BigEndian.Uint16(authData[53:55]))
	if n > 1023 || len(authData) < 55+n || subtle.ConstantTimeCompare(authData[55:55+n], id) != 1 {
		failed(w, http.StatusBadRequest, errors.New("the passkey's ID is not the one the authenticator made"))
		return
	}
	if body.Algorithm != es256 {
		failed(w, http.StatusBadRequest, fmt.Errorf("the passkey's algorithm %d is not ES256 (-7)", body.Algorithm))
		return
	}
	if _, err := p256Key(spki); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	now := s.now().UTC()
	pk := &config.Passkey{ID: rawKey.EncodeToString(id), PublicKey: rawKey.EncodeToString(spki), Algorithm: es256, RPID: rpID, SignCount: count, Added: now, Checked: now}
	if err := s.updateDevice(d.ID, func(cur *config.Device) error {
		if cur.Passkey != nil {
			return errHasPasskey
		}
		cur.Passkey = pk
		return nil
	}); err != nil {
		failed(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"until": now.Add(checkLife)})
}

var errHasPasskey = errors.New("this phone has a passkey already")

// passkeyVerify checks the phone's passkey: the ceremony, the signature with the kept
// key over the authenticator data and the client data's hash, and the counter, which
// may not go back nor stand still, as a copied passkey's would, unless it is never
// counted (zero before and now, as Apple's synced passkeys are).
func (s *Server) passkeyVerify(w http.ResponseWriter, r *http.Request, d config.Device) {
	var body passkeyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		failed(w, http.StatusBadRequest, err)
		return
	}
	value, err := s.takeChallenge(d.ID, "webauthn.get")
	if err != nil {
		failed(w, http.StatusForbidden, err)
		return
	}
	rpID, origin := s.relyingParty()
	refuse := func(err error) { failed(w, http.StatusForbidden, err) }
	if d.Passkey == nil || d.Passkey.RPID != rpID {
		refuse(errors.New("this phone has no passkey for this computer's name"))
		return
	}
	parts, err := decode(body.ID, body.ClientDataJSON, body.AuthenticatorData, body.Signature)
	if err != nil {
		refuse(err)
		return
	}
	id, clientData, authData, sig := parts[0], parts[1], parts[2], parts[3]
	if subtle.ConstantTimeCompare([]byte(rawKey.EncodeToString(id)), []byte(d.Passkey.ID)) != 1 {
		refuse(errors.New("that is not this phone's passkey"))
		return
	}
	count, err := ceremony(clientData, authData, "webauthn.get", value, rpID, origin)
	if err != nil {
		refuse(err)
		return
	}
	spki, err := base64.RawURLEncoding.DecodeString(d.Passkey.PublicKey)
	if err != nil {
		refuse(errors.New("the kept public key is damaged"))
		return
	}
	pub, err := p256Key(spki)
	if err != nil {
		refuse(err)
		return
	}
	clientHash := sha256.Sum256(clientData)
	signed := sha256.Sum256(append(append([]byte{}, authData...), clientHash[:]...))
	if !ecdsa.VerifyASN1(pub, signed[:], sig) {
		refuse(errors.New("the passkey's signature does not match"))
		return
	}
	now := s.now().UTC()
	if err := s.updateDevice(d.ID, func(cur *config.Device) error {
		if cur.Passkey == nil || cur.Passkey.ID != d.Passkey.ID {
			return errors.New("this phone's passkey changed meanwhile")
		}
		if (count != 0 || cur.Passkey.SignCount != 0) && count <= cur.Passkey.SignCount {
			return errors.New("the passkey's counter went back: it may have been copied; remove the phone and pair it again")
		}
		pk := *cur.Passkey
		pk.SignCount, pk.Checked = count, now
		cur.Passkey = &pk
		return nil
	}); err != nil {
		refuse(err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"until": now.Add(checkLife)})
}

// updateDevice changes the phone id's record with change and saves the list, all under
// s.mu; a record is replaced, never changed in place, as callers hold copies.
func (s *Server) updateDevice(id string, change func(*config.Device) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := append([]config.Device{}, s.devices...)
	for i := range next {
		if next[i].ID != id {
			continue
		}
		if err := change(&next[i]); err != nil {
			return err
		}
		if err := config.SaveDevices(s.dataDir, next); err != nil {
			return err
		}
		s.devices = next
		return nil
	}
	return errors.New("no such phone")
}
