import { test } from "node:test";
import assert from "node:assert/strict";
import { assertion, cancelled, creationOptions, fromB64url, OldBrowser, passkeysSupported, registration, requestOptions, toB64url } from "./passkey.ts";
import { api, PasskeyNeeded, passkeyCeremony, Unpaired } from "./api.ts";

const bytes = (...b: number[]) => Uint8Array.from(b).buffer;

test("bytes go to the server and back as base64url, as it writes them", () => {
  const all = Uint8Array.from({ length: 256 }, (_, i) => i);
  assert.deepEqual(Array.from(fromB64url(toB64url(all))), Array.from(all));
  assert.equal(toB64url(bytes(0xfb, 0xff)), "-_8");
  assert.deepEqual(Array.from(fromB64url("-_8")), [0xfb, 0xff]);
  assert.deepEqual(Array.from(fromB64url("-_8=")), [0xfb, 0xff]);
  // A view of part of a buffer is only that part.
  assert.equal(toB64url(new Uint8Array(Uint8Array.from([9, 1, 2, 9]).buffer, 1, 2)), "AQI");
});

// OQ-052: the passkey is the phone's own, its user verified, ES256, with no attestation;
// a check asks for that passkey alone, its user verified.
test("a passkey is asked of the phone itself, with its user verified", () => {
  const o = creationOptions({ challenge: "AQID", rpId: "laptop.tail1234.ts.net", userId: toB64url(new TextEncoder().encode("dev_1")), userName: "iPhone", timeout: 120000 });
  assert.deepEqual(Array.from(o.challenge as Uint8Array), [1, 2, 3]);
  assert.deepEqual(o.rp, { id: "laptop.tail1234.ts.net", name: "Freeloop" });
  assert.equal(new TextDecoder().decode(o.user.id as Uint8Array), "dev_1");
  assert.equal(o.user.name, "iPhone");
  assert.deepEqual(o.pubKeyCredParams, [{ type: "public-key", alg: -7 }]);
  assert.equal(o.authenticatorSelection?.authenticatorAttachment, "platform");
  assert.equal(o.authenticatorSelection?.userVerification, "required");
  assert.equal(o.attestation, "none");
  const g = requestOptions({ challenge: "BAU", rpId: "laptop.tail1234.ts.net", credentialId: "Y3JlZA", timeout: 120000 });
  assert.deepEqual(Array.from(g.challenge as Uint8Array), [4, 5]);
  assert.equal(g.rpId, "laptop.tail1234.ts.net");
  assert.equal(g.userVerification, "required");
  assert.equal(g.allowCredentials?.length, 1);
  assert.equal(new TextDecoder().decode(g.allowCredentials![0].id as Uint8Array), "cred");
});

test("a new passkey goes to the server with its public key, which an old browser does not give", () => {
  const made = {
    rawId: bytes(1, 2),
    response: { clientDataJSON: bytes(3), getAuthenticatorData: () => bytes(4), getPublicKey: () => bytes(5, 6), getPublicKeyAlgorithm: () => -7 },
  };
  assert.deepEqual(registration(made), { id: "AQI", clientDataJSON: "Aw", authenticatorData: "BA", publicKey: "BQY", algorithm: -7 });
  assert.throws(() => registration({ rawId: bytes(1), response: { clientDataJSON: bytes(3) } }), OldBrowser);
  // A key in an algorithm the browser cannot write as SPKI comes as null.
  assert.throws(() => registration({ ...made, response: { ...made.response, getPublicKey: () => null } }), OldBrowser);
  const shown = { rawId: bytes(1, 2), response: { clientDataJSON: bytes(3), authenticatorData: bytes(4), signature: bytes(7) } };
  assert.deepEqual(assertion(shown), { id: "AQI", clientDataJSON: "Aw", authenticatorData: "BA", signature: "Bw" });
});

test("passkeys need WebAuthn in a secure page, and a no is not a fault", () => {
  assert.equal(passkeysSupported({}), false);
  assert.equal(passkeysSupported({ PublicKeyCredential: class {}, credentials: {}, isSecureContext: true }), true);
  assert.equal(passkeysSupported({ PublicKeyCredential: class {}, credentials: {}, isSecureContext: false }), false);
  assert.equal(cancelled(new DOMException("no", "NotAllowedError")), true);
  assert.equal(cancelled(new DOMException("gone", "AbortError")), true);
  assert.equal(cancelled(new DOMException("bad", "SecurityError")), false);
  assert.equal(cancelled(null), false);
});

// A computer that answers each call with the next of answers, and keeps what was sent.
function computer(answers: { status: number; body: unknown }[]) {
  const sent: { url: string; body: unknown }[] = [];
  const real = { fetch: globalThis.fetch, storage: (globalThis as { localStorage?: unknown }).localStorage };
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    sent.push({ url, body: init.body ? JSON.parse(String(init.body)) : undefined });
    const a = answers.shift()!;
    return new Response(JSON.stringify(a.body), { status: a.status });
  }) as typeof fetch;
  (globalThis as { localStorage?: unknown }).localStorage = { getItem: () => "key", removeItem: () => {}, setItem: () => {} };
  return {
    sent,
    restore: () => {
      globalThis.fetch = real.fetch;
      (globalThis as { localStorage?: unknown }).localStorage = real.storage;
    },
  };
}

test("an answer the server holds back for the passkey says which it needs; a lost key is still a lost key", async () => {
  const c = computer([
    { status: 403, body: { passkey: "stale", error: "show this phone's passkey first" } },
    { status: 403, body: { passkey: "missing", error: "no passkey" } },
    { status: 403, body: { error: "answer this on the computer" } },
    { status: 401, body: {} },
  ]);
  try {
    await assert.rejects(api.decide("perm_1", "allow_once"), (err) => err instanceof PasskeyNeeded && err.need === "stale");
    await assert.rejects(api.prompt("ses_1", "hello"), (err) => err instanceof PasskeyNeeded && err.need === "missing");
    await assert.rejects(api.decide("perm_1", "allow_always"), (err) => !(err instanceof PasskeyNeeded) && (err as Error).message === "answer this on the computer");
    await assert.rejects(api.decide("perm_1", "allow_once"), Unpaired);
  } finally {
    c.restore();
  }
});

test("the browser is asked at once, from the tap, and its answer goes to the server", async () => {
  const c = computer([{ status: 200, body: { until: "2026-11-26T08:00:00Z" } }, { status: 200, body: { until: "2026-11-26T09:00:00Z" } }]);
  try {
    const asked: string[] = [];
    const creds = {
      create: async (o?: CredentialCreationOptions) => {
        asked.push(`create ${o?.publicKey?.rp.id}`);
        return { rawId: bytes(1), response: { clientDataJSON: bytes(2), getAuthenticatorData: () => bytes(3), getPublicKey: () => bytes(4), getPublicKeyAlgorithm: () => -7 } } as unknown as Credential;
      },
      get: async (o?: CredentialRequestOptions) => {
        asked.push(`get ${o?.publicKey?.rpId}`);
        return { rawId: bytes(1), response: { clientDataJSON: bytes(2), authenticatorData: bytes(3), signature: bytes(5) } } as unknown as Credential;
      },
    };
    const made = passkeyCeremony({ create: { challenge: "AQ", rpId: "host.ts.net", userId: "ZA", userName: "iPhone", timeout: 1000 } }, creds);
    // Before anything is awaited: Safari asks for a passkey only straight from a tap.
    assert.deepEqual(asked, ["create host.ts.net"]);
    assert.deepEqual(await made, { until: "2026-11-26T08:00:00Z" });
    const shown = passkeyCeremony({ get: { challenge: "AQ", rpId: "host.ts.net", credentialId: "AQ", timeout: 1000 } }, creds);
    assert.deepEqual(asked, ["create host.ts.net", "get host.ts.net"]);
    await shown;
    assert.deepEqual(c.sent, [
      { url: "/api/passkey/register", body: { id: "AQ", clientDataJSON: "Ag", authenticatorData: "Aw", publicKey: "BA", algorithm: -7 } },
      { url: "/api/passkey/verify", body: { id: "AQ", clientDataJSON: "Ag", authenticatorData: "Aw", signature: "BQ" } },
    ]);
    // No credential is the user's no.
    await assert.rejects(passkeyCeremony({ get: { challenge: "AQ", rpId: "h", credentialId: "AQ", timeout: 1 } }, { ...creds, get: async () => null }), (err) => cancelled(err));
  } finally {
    c.restore();
  }
});
