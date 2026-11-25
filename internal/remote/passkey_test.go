package remote

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
)

// authenticator is a passkey in software: what a phone's browser and its authenticator
// hand the phone app for a challenge, built as they build it (WebAuthn level 2). Its
// fields are what a test bends to make one of them wrong.
type authenticator struct {
	key    *ecdsa.PrivateKey
	id     []byte
	count  uint32
	rpID   string
	origin string
	flags  byte
	// kind overrides the client data's type when set.
	kind        string
	crossOrigin bool
}

func newAuthenticator(t *testing.T) *authenticator {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	rand.Read(id)
	return &authenticator{key: k, id: id, rpID: "laptop.tail1234.ts.net", origin: "https://laptop.tail1234.ts.net:8443", flags: flagUP | flagUV}
}

func enc(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (a *authenticator) clientData(kind, challenge string) []byte {
	if a.kind != "" {
		kind = a.kind
	}
	b, _ := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": a.origin, "crossOrigin": a.crossOrigin})
	return b
}

// authData is the authenticator's data: the RP ID's hash, the flags, the counter and, at
// registration, the credential with its key in COSE form.
func (a *authenticator) authData(attested bool) []byte {
	h := sha256.Sum256([]byte(a.rpID))
	out := append([]byte{}, h[:]...)
	flags := a.flags
	if attested {
		flags |= flagAT
	}
	out = append(out, flags)
	out = binary.BigEndian.AppendUint32(out, a.count)
	if attested {
		out = append(out, make([]byte, 16)...) // no AAGUID, as with attestation "none"
		out = binary.BigEndian.AppendUint16(out, uint16(len(a.id)))
		out = append(out, a.id...)
		pub, _ := a.key.PublicKey.Bytes()
		// {1: 2, 3: -7, -1: 1, -2: x, -3: y}, as CTAP2 writes an ES256 key.
		out = append(out, 0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20)
		out = append(out, pub[1:33]...)
		out = append(out, 0x22, 0x58, 0x20)
		out = append(out, pub[33:]...)
	}
	return out
}

// create answers a registration challenge, as the phone app reads what
// navigator.credentials.create gives (getPublicKey, getPublicKeyAlgorithm).
func (a *authenticator) create(challenge string) map[string]any {
	spki, _ := x509.MarshalPKIXPublicKey(&a.key.PublicKey)
	return map[string]any{"id": enc(a.id), "clientDataJSON": enc(a.clientData("webauthn.create", challenge)), "authenticatorData": enc(a.authData(true)), "publicKey": enc(spki), "algorithm": -7}
}

// get answers a check's challenge, as navigator.credentials.get does: the signature is
// over the authenticator's data and the hash of the client data.
func (a *authenticator) get(challenge string) map[string]any {
	cd := a.clientData("webauthn.get", challenge)
	ad := a.authData(false)
	sum := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), sum[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	return map[string]any{"id": enc(a.id), "clientDataJSON": enc(cd), "authenticatorData": enc(ad), "signature": enc(sig)}
}

// challenge asks the server for one, as the phone app does before it calls the
// browser; kind is "create" or "get", whichever the server gave.
func (p *phone) challenge() (kind, value string) {
	p.t.Helper()
	resp, out := p.do("POST", "/api/passkey/challenge", map[string]any{}, nil)
	if resp.StatusCode != http.StatusOK {
		p.t.Fatalf("challenge: %d %v", resp.StatusCode, out)
	}
	for _, kind := range []string{"create", "get"} {
		if o, ok := out[kind].(map[string]any); ok {
			v, _ := o["challenge"].(string)
			return kind, v
		}
	}
	p.t.Fatalf("challenge: %v", out)
	return "", ""
}

// addPasskey makes the phone's passkey, as the phone app does once it is paired.
func (p *phone) addPasskey() {
	p.t.Helper()
	if p.passkey == nil {
		p.passkey = newAuthenticator(p.t)
	}
	kind, ch := p.challenge()
	if kind != "create" {
		p.t.Fatalf("a phone without a passkey was given a %s challenge", kind)
	}
	if resp, out := p.do("POST", "/api/passkey/register", p.passkey.create(ch), nil); resp.StatusCode != http.StatusOK {
		p.t.Fatalf("register: %d %v", resp.StatusCode, out)
	}
}

// showPasskey answers a check with the phone's passkey.
func (p *phone) showPasskey() (*http.Response, map[string]any) {
	p.t.Helper()
	kind, ch := p.challenge()
	if kind != "get" {
		p.t.Fatalf("a phone with a passkey was given a %s challenge", kind)
	}
	return p.do("POST", "/api/passkey/verify", p.passkey.get(ch), nil)
}

// gate says how the server takes an answer and a message from the phone: "" when they
// get past the check (to a request and a session that do not exist), or what it needs.
func (p *phone) gate() (decide, prompt string) {
	p.t.Helper()
	need := func(resp *http.Response, out map[string]any, past int) string {
		if resp.StatusCode == past {
			return ""
		}
		if n, _ := out["passkey"].(string); resp.StatusCode == http.StatusForbidden && n != "" {
			return n
		}
		p.t.Fatalf("%d %v", resp.StatusCode, out)
		return ""
	}
	resp, out := p.do("POST", "/api/decide", map[string]string{"requestId": "perm_none", "optionId": "allow_once"}, nil)
	decide = need(resp, out, http.StatusConflict)
	resp, out = p.do("POST", "/api/sessions/ses_none/prompt", map[string]string{"text": "hello"}, nil)
	prompt = need(resp, out, http.StatusNotFound)
	return decide, prompt
}

// clock lets a test move the server's time on.
func clock(s *Server) *atomic.Int64 {
	var skew atomic.Int64
	s.now = func() time.Time { return time.Now().Add(time.Duration(skew.Load())) }
	return &skew
}

// OQ-052: a phone answers a request or messages a session only with a passkey check
// less than 18 hours old. One paired before passkeys has none and must add one first;
// reading and stopping need only its key. Making the passkey is a check; after 18
// hours the phone shows it again. Settings says which phones have one.
func TestAnswersFromThePhoneNeedAPasskeyCheckWithin18Hours(t *testing.T) {
	_, s, p := started(t)
	skew := clock(s)
	pairOnly(t, s, p)
	if resp, _ := p.do("GET", "/api/summary", nil, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("reading without a passkey: %d", resp.StatusCode)
	}
	if resp, _ := p.do("POST", "/api/sessions/ses_none/cancel", map[string]any{}, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("stopping without a passkey: %d", resp.StatusCode)
	}
	if d, m := p.gate(); d != "missing" || m != "missing" {
		t.Fatalf("a phone without a passkey: answer %q, message %q", d, m)
	}
	if _, out := p.do("GET", "/api/passkey", nil, nil); out["need"] != "missing" {
		t.Errorf("its passkey state: %v", out)
	}
	if st := s.Status(); len(st.Devices) != 1 || st.Devices[0].Passkey {
		t.Errorf("Settings: %+v", st.Devices)
	}

	p.addPasskey()
	if d, m := p.gate(); d != "" || m != "" {
		t.Fatalf("just after making its passkey: answer %q, message %q", d, m)
	}
	_, out := p.do("GET", "/api/passkey", nil, nil)
	until, _ := time.Parse(time.RFC3339Nano, out["until"].(string))
	if out["need"] != "" || until.Sub(s.now()) < checkLife-time.Minute || until.Sub(s.now()) > checkLife {
		t.Errorf("its passkey state: %v", out)
	}
	saved, _ := config.LoadDevices(s.dataDir)
	if len(saved) != 1 || saved[0].Passkey == nil || saved[0].Passkey.ID != enc(p.passkey.id) || saved[0].Passkey.RPID != "laptop.tail1234.ts.net" || saved[0].Passkey.Algorithm != -7 {
		t.Fatalf("the saved passkey: %+v", saved)
	}
	if pub, err := x509.ParsePKIXPublicKey(must(base64.RawURLEncoding.DecodeString(saved[0].Passkey.PublicKey))); err != nil || !pub.(*ecdsa.PublicKey).Equal(&p.passkey.key.PublicKey) {
		t.Errorf("the saved key is not the phone's: %v", err)
	}
	if st := s.Status(); !st.Devices[0].Passkey {
		t.Errorf("Settings: %+v", st.Devices)
	}

	skew.Store(int64(checkLife - time.Minute))
	if d, m := p.gate(); d != "" || m != "" {
		t.Errorf("a minute before 18 hours: answer %q, message %q", d, m)
	}
	skew.Store(int64(checkLife))
	if d, m := p.gate(); d != "stale" || m != "stale" {
		t.Fatalf("after 18 hours: answer %q, message %q", d, m)
	}
	if resp, out := p.showPasskey(); resp.StatusCode != http.StatusOK {
		t.Fatalf("the check: %d %v", resp.StatusCode, out)
	}
	if d, m := p.gate(); d != "" || m != "" {
		t.Errorf("after the check: answer %q, message %q", d, m)
	}
	// Measured from the last check, not the first.
	skew.Store(int64(2*checkLife - time.Minute))
	if d, _ := p.gate(); d != "" {
		t.Errorf("17 hours 59 after the second check: %q", d)
	}
	// A clock put back does not stretch a check.
	skew.Store(-int64(time.Hour))
	if d, _ := p.gate(); d != "stale" {
		t.Errorf("with the clock put back: %q", d)
	}
}

// What the server checks of a passkey check (SEC-020): the challenge it gave (once, and
// for a short time), the page and the name it was made for, that the phone verified its
// user, the signature with the kept key, and the counter. A check that fails leaves the
// phone as it was.
func TestAPasskeyCheckIsRefusedWhenAnyPartIsWrong(t *testing.T) {
	_, s, p := started(t)
	skew := clock(s)
	pairPhone(t, s, p)
	skew.Store(int64(checkLife))
	good := *p.passkey
	other := newAuthenticator(t)
	for _, c := range []struct {
		name string
		bend func(a *authenticator)
		body func(map[string]any)
	}{
		{name: "made on another site", bend: func(a *authenticator) { a.origin = "https://evil.example" }},
		{name: "made on another port of the same name", bend: func(a *authenticator) { a.origin = "https://laptop.tail1234.ts.net" }},
		{name: "made over plain HTTP", bend: func(a *authenticator) { a.origin = "http://laptop.tail1234.ts.net:8443" }},
		{name: "made in a frame", bend: func(a *authenticator) { a.crossOrigin = true }},
		{name: "for another name", bend: func(a *authenticator) { a.rpID = "evil.example" }},
		{name: "a registration's client data", bend: func(a *authenticator) { a.kind = "webauthn.create" }},
		{name: "without the user verified", bend: func(a *authenticator) { a.flags = flagUP }},
		{name: "without the user present", bend: func(a *authenticator) { a.flags = flagUV }},
		{name: "signed by another key", bend: func(a *authenticator) { a.key = other.key }},
		{name: "of another passkey", bend: func(a *authenticator) { a.id = other.id }},
		{name: "a signature that does not match", body: func(b map[string]any) {
			sig := must(base64.RawURLEncoding.DecodeString(b["signature"].(string)))
			sig[len(sig)-1] ^= 1
			b["signature"] = enc(sig)
		}},
		{name: "client data changed after signing", body: func(b map[string]any) {
			var cd map[string]any
			json.Unmarshal(must(base64.RawURLEncoding.DecodeString(b["clientDataJSON"].(string))), &cd)
			cd["extra"] = 1
			raw, _ := json.Marshal(cd)
			b["clientDataJSON"] = enc(raw)
		}},
		{name: "not base64", body: func(b map[string]any) { b["authenticatorData"] = "!!" }},
		{name: "short authenticator data", body: func(b map[string]any) { b["authenticatorData"] = enc([]byte{1, 2, 3}) }},
	} {
		a := good
		if c.bend != nil {
			c.bend(&a)
		}
		_, ch := p.challenge()
		body := a.get(ch)
		if c.body != nil {
			c.body(body)
		}
		if resp, out := p.do("POST", "/api/passkey/verify", body, nil); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d %v", c.name, resp.StatusCode, out)
		}
		if d, _ := p.gate(); d != "stale" {
			t.Errorf("%s: the phone may answer after it: %q", c.name, d)
		}
	}

	// A challenge it did not give, and none at all.
	_, ch := p.challenge()
	if resp, _ := p.do("POST", "/api/passkey/verify", p.passkey.get(enc([]byte("not the server's challenge"))), nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a challenge the server did not give: %d", resp.StatusCode)
	}
	if resp, _ := p.do("POST", "/api/passkey/verify", p.passkey.get(ch), nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("the challenge a failed check used, again: %d", resp.StatusCode)
	}
	// One that has been answered is not taken twice.
	_, ch = p.challenge()
	replay := p.passkey.get(ch)
	if resp, out := p.do("POST", "/api/passkey/verify", replay, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("a good check: %d %v", resp.StatusCode, out)
	}
	if resp, _ := p.do("POST", "/api/passkey/verify", replay, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("the same check again: %d", resp.StatusCode)
	}
	// One left too long is refused.
	skew.Add(int64(checkLife))
	_, ch = p.challenge()
	skew.Add(int64(challengeLife + time.Second))
	if resp, _ := p.do("POST", "/api/passkey/verify", p.passkey.get(ch), nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a challenge older than %s: %d", challengeLife, resp.StatusCode)
	}
	// A newer challenge replaces the one before.
	_, first := p.challenge()
	p.challenge()
	if resp, _ := p.do("POST", "/api/passkey/verify", p.passkey.get(first), nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a challenge a newer one replaced: %d", resp.StatusCode)
	}

	// The counter: a passkey that counts may not count back or stand still, which a
	// copied one does; one that never counts (Apple's, synced) stays at zero.
	p.passkey.count = 5
	if resp, out := p.showPasskey(); resp.StatusCode != http.StatusOK {
		t.Fatalf("counter 5: %d %v", resp.StatusCode, out)
	}
	for _, n := range []uint32{5, 3, 0} {
		p.passkey.count = n
		skew.Add(int64(checkLife))
		if resp, _ := p.showPasskey(); resp.StatusCode != http.StatusForbidden {
			t.Errorf("counter %d after 5: %d", n, resp.StatusCode)
		}
	}
	p.passkey.count = 6
	if resp, _ := p.showPasskey(); resp.StatusCode != http.StatusOK {
		t.Errorf("counter 6 after 5: %d", resp.StatusCode)
	}
	if d, _ := config.LoadDevices(s.dataDir); d[0].Passkey.SignCount != 6 {
		t.Errorf("the saved counter: %d", d[0].Passkey.SignCount)
	}
	_, s2, p2 := started(t)
	pairPhone(t, s2, p2)
	for range 2 {
		if resp, out := p2.showPasskey(); resp.StatusCode != http.StatusOK {
			t.Errorf("a counter that stays at zero: %d %v", resp.StatusCode, out)
		}
	}
}

// Making a passkey is checked as a check is, and takes only a P-256 key (ES256). A phone
// that has one cannot make another with its key alone: that would let whoever holds the
// key replace it. A new one comes with pairing the phone again.
func TestAPasskeyIsMadeOnceAndChecked(t *testing.T) {
	_, s, p := started(t)
	skew := clock(s)
	pairOnly(t, s, p)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	for _, c := range []struct {
		name string
		bend func(a *authenticator)
		body func(map[string]any)
	}{
		{name: "made on another site", bend: func(a *authenticator) { a.origin = "https://evil.example" }},
		{name: "for another name", bend: func(a *authenticator) { a.rpID = "laptop.other.ts.net" }},
		{name: "a check's client data", bend: func(a *authenticator) { a.kind = "webauthn.get" }},
		{name: "without the user verified", bend: func(a *authenticator) { a.flags = flagUP }},
		{name: "another algorithm", body: func(b map[string]any) { b["algorithm"] = -257 }},
		{name: "a P-384 key", body: func(b map[string]any) {
			spki, _ := x509.MarshalPKIXPublicKey(&p384.PublicKey)
			b["publicKey"] = enc(spki)
		}},
		{name: "not a key", body: func(b map[string]any) { b["publicKey"] = enc([]byte("key")) }},
		{name: "another credential than the authenticator's", body: func(b map[string]any) { b["id"] = enc([]byte("someone else's")) }},
		{name: "no credential in the authenticator's data", body: func(b map[string]any) {
			ad := must(base64.RawURLEncoding.DecodeString(b["authenticatorData"].(string)))
			ad[32] &^= flagAT
			b["authenticatorData"] = enc(ad[:37])
		}},
	} {
		a := newAuthenticator(t)
		if c.bend != nil {
			c.bend(a)
		}
		_, ch := p.challenge()
		body := a.create(ch)
		if c.body != nil {
			c.body(body)
		}
		if resp, out := p.do("POST", "/api/passkey/register", body, nil); resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d %v", c.name, resp.StatusCode, out)
		}
		if d, _ := p.gate(); d != "missing" {
			t.Errorf("%s: kept: %q", c.name, d)
		}
	}
	_, ch := p.challenge()
	skew.Add(int64(challengeLife + time.Second))
	if resp, _ := p.do("POST", "/api/passkey/register", newAuthenticator(t).create(ch), nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a challenge older than %s: %d", challengeLife, resp.StatusCode)
	}

	p.addPasskey()
	if kind, _ := p.challenge(); kind != "get" {
		t.Errorf("a phone with a passkey is asked to make one: %s", kind)
	}
	// Its challenge is for a check, so a new passkey for it is refused.
	_, ch = p.challenge()
	if resp, _ := p.do("POST", "/api/passkey/register", newAuthenticator(t).create(ch), nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("a second passkey: %d", resp.StatusCode)
	}
	if d, _ := config.LoadDevices(s.dataDir); d[0].Passkey.ID != enc(p.passkey.id) {
		t.Errorf("the passkey was replaced: %+v", d[0].Passkey)
	}
}

// Removing a phone removes its passkey, and a challenge it was given goes with it.
// A passkey made for another name than the one now set is no use: the phone pairs again.
func TestAPasskeyGoesWithItsPhoneAndItsName(t *testing.T) {
	e, s, p := started(t)
	pairPhone(t, s, p)
	_, ch := p.challenge()
	if err := s.RemoveDevice(s.Status().Devices[0].ID); err != nil {
		t.Fatal(err)
	}
	if d, _ := config.LoadDevices(s.dataDir); len(d) != 0 {
		t.Fatalf("a removed phone is kept: %+v", d)
	}
	s.mu.Lock()
	left := len(s.challenges)
	s.mu.Unlock()
	if left != 0 {
		t.Errorf("a removed phone's challenge is kept")
	}
	if resp, _ := p.do("POST", "/api/passkey/verify", p.passkey.get(ch), nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a removed phone's check: %d", resp.StatusCode)
	}

	p.passkey = nil
	pairPhone(t, s, p)
	cfg := e.Config()
	cfg.Remote.Host = "laptop.other.ts.net"
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	s.apply(e.RemoteState())
	if d, m := p.gate(); d != "pair-again" || m != "pair-again" {
		t.Errorf("a passkey for the name before: answer %q, message %q", d, m)
	}
	if resp, out := p.do("POST", "/api/passkey/challenge", map[string]any{}, nil); resp.StatusCode != http.StatusConflict || out["passkey"] != "pair-again" {
		t.Errorf("a challenge for it: %d %v", resp.StatusCode, out)
	}
	// With no name set there is nothing to make a passkey for.
	cfg.Remote.Host = ""
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	s.apply(e.RemoteState())
	if rp, _ := s.relyingParty(); rp != "" {
		t.Errorf("relying party with no name: %q", rp)
	}
	if resp, out := p.do("POST", "/api/passkey/challenge", map[string]any{}, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("a challenge with no name set: %d %v", resp.StatusCode, out)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
