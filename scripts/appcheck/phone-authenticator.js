// A stand-in for the phone's passkeys (OQ-052), put before the phone app's own scripts
// (APPCHECK_INIT). The check's WebKit view has no authenticator, and serves the phone app
// from 127.0.0.1, where WebAuthn would not make a passkey for the tailnet name anyway. So
// navigator.credentials makes and shows passkeys in software, with WebCrypto, as a phone's
// browser and authenticator do on the tailnet name: the client data names the page
// https://<rpId>:8443, the user is present and verified, the key is ES256 and the counter
// stays at zero, as Apple's synced passkeys do. The server checks what it makes as it
// checks a real phone's. What stands in here is the browser and Face ID, nothing of the
// app: window.__passkeys keeps the options each call was given, and the private keys live
// in the page's storage under appcheck.passkeys, as a phone keeps its own.
(() => {
  const enc = new TextEncoder();
  const b64url = (b) => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  const concat = (...parts) => {
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let i = 0;
    for (const p of parts) {
      out.set(p, i);
      i += p.length;
    }
    return out;
  };
  const sha256 = async (b) => new Uint8Array(await crypto.subtle.digest("SHA-256", b));
  // WebCrypto signs as r and s side by side; WebAuthn's signature is their ASN.1 DER.
  const der = (raw) => {
    const int = (b) => {
      let i = 0;
      while (i < b.length - 1 && b[i] === 0 && !(b[i + 1] & 0x80)) i++;
      b = b.slice(i);
      if (b[0] & 0x80) b = concat([0], b);
      return concat([2, b.length], b);
    };
    const r = int(raw.slice(0, 32));
    const s = int(raw.slice(32));
    return concat([0x30, r.length + s.length], r, s);
  };
  const keys = () => JSON.parse(localStorage.getItem("appcheck.passkeys") || "{}");
  const clientData = (type, challenge, rpId) => enc.encode(JSON.stringify({ type, challenge: b64url(challenge), origin: `https://${rpId}:8443`, crossOrigin: false }));
  window.__passkeys = { created: [], got: [] };
  const credentials = {
    async create({ publicKey: o }) {
      window.__passkeys.created.push(o);
      const pair = await crypto.subtle.generateKey({ name: "ECDSA", namedCurve: "P-256" }, true, ["sign", "verify"]);
      const id = crypto.getRandomValues(new Uint8Array(16));
      localStorage.setItem("appcheck.passkeys", JSON.stringify({ ...keys(), [b64url(id)]: await crypto.subtle.exportKey("jwk", pair.privateKey) }));
      const spki = new Uint8Array(await crypto.subtle.exportKey("spki", pair.publicKey));
      const point = new Uint8Array(await crypto.subtle.exportKey("raw", pair.publicKey));
      // {1: 2, 3: -7, -1: 1, -2: x, -3: y}, as CTAP2 writes an ES256 key.
      const cose = concat([0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20], point.slice(1, 33), [0x22, 0x58, 0x20], point.slice(33));
      // Present, verified, a credential follows; counter 0; no AAGUID.
      const authData = concat(await sha256(enc.encode(o.rp.id)), [0x45, 0, 0, 0, 0], new Uint8Array(16), [0, id.length], id, cose);
      const cd = clientData("webauthn.create", new Uint8Array(o.challenge), o.rp.id);
      return {
        id: b64url(id),
        rawId: id.buffer,
        type: "public-key",
        response: { clientDataJSON: cd.buffer, getAuthenticatorData: () => authData.buffer, getPublicKey: () => spki.buffer, getPublicKeyAlgorithm: () => -7 },
      };
    },
    async get({ publicKey: o }) {
      window.__passkeys.got.push(o);
      const id = new Uint8Array(o.allowCredentials[0].id);
      const jwk = keys()[b64url(id)];
      if (!jwk) throw new DOMException("this phone has no such passkey", "NotAllowedError");
      const key = await crypto.subtle.importKey("jwk", jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["sign"]);
      const authData = concat(await sha256(enc.encode(o.rpId)), [0x05, 0, 0, 0, 0]);
      const cd = clientData("webauthn.get", new Uint8Array(o.challenge), o.rpId);
      const sig = new Uint8Array(await crypto.subtle.sign({ name: "ECDSA", hash: "SHA-256" }, key, concat(authData, await sha256(cd))));
      return { id: b64url(id), rawId: id.buffer, type: "public-key", response: { clientDataJSON: cd.buffer, authenticatorData: authData.buffer, signature: der(sig).buffer } };
    },
  };
  Object.defineProperty(navigator, "credentials", { configurable: true, get: () => credentials });
  if (!window.PublicKeyCredential) window.PublicKeyCredential = function PublicKeyCredential() {};
})();
