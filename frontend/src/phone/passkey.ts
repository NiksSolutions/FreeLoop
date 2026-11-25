// The phone's passkey (OQ-052, SEC-020): made at pairing for the computer's tailnet name,
// and shown again, with Face ID, Touch ID or the phone's code, before an answer or a
// message once the last check is 18 hours old (the server, internal/remote/passkey.go,
// measures and checks). What the browser's WebAuthn takes and gives is turned to and from
// what the server sends here; pure, so node --test covers it.

// Need is what the server asks of the phone before an answer or a message: nothing, a
// passkey to make, a check because the last is too old, or pairing again, as its passkey
// is for another name of the computer.
export type Need = "" | "missing" | "stale" | "pair-again";

export interface CreateChallenge {
  challenge: string;
  rpId: string;
  userId: string;
  userName: string;
  timeout: number;
}

export interface GetChallenge {
  challenge: string;
  rpId: string;
  credentialId: string;
  timeout: number;
}

// Challenge is the server's: to make the passkey, or to show it.
export type Challenge = { create: CreateChallenge } | { get: GetChallenge };

// fromB64url reads base64url, as the server writes bytes, padded or not.
export function fromB64url(s: string): Uint8Array<ArrayBuffer> {
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/").replace(/=+$/, "");
  const raw = atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4));
  return Uint8Array.from(raw, (c) => c.charCodeAt(0));
}

// toB64url writes bytes as base64url without padding, as the server reads them.
export function toB64url(b: ArrayBuffer | ArrayBufferView): string {
  const bytes = b instanceof ArrayBuffer ? new Uint8Array(b) : new Uint8Array(b.buffer, b.byteOffset, b.byteLength);
  let raw = "";
  for (const c of bytes) raw += String.fromCharCode(c);
  return btoa(raw).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// creationOptions make the passkey on this phone (a platform authenticator), its user
// verified, ES256 only, with no attestation: the server keeps only its public key.
export function creationOptions(c: CreateChallenge): PublicKeyCredentialCreationOptions {
  return {
    challenge: fromB64url(c.challenge),
    rp: { id: c.rpId, name: "Freeloop" },
    user: { id: fromB64url(c.userId), name: c.userName, displayName: c.userName },
    pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    authenticatorSelection: { authenticatorAttachment: "platform", residentKey: "preferred", userVerification: "required" },
    attestation: "none",
    timeout: c.timeout,
  };
}

// requestOptions ask for this phone's passkey and no other, its user verified.
export function requestOptions(c: GetChallenge): PublicKeyCredentialRequestOptions {
  return {
    challenge: fromB64url(c.challenge),
    rpId: c.rpId,
    allowCredentials: [{ type: "public-key", id: fromB64url(c.credentialId) }],
    userVerification: "required",
    timeout: c.timeout,
  };
}

// OldBrowser is a browser that makes a passkey but does not hand over its public key
// (WebAuthn level 2's getPublicKey), which the server needs, reading no CBOR.
export class OldBrowser extends Error {}

// What the phone app reads of a credential, as the browser's PublicKeyCredential has it.
export interface MadeCredential {
  rawId: ArrayBuffer;
  response: {
    clientDataJSON: ArrayBuffer;
    getAuthenticatorData?: () => ArrayBuffer;
    getPublicKey?: () => ArrayBuffer | null;
    getPublicKeyAlgorithm?: () => number;
  };
}

export interface ShownCredential {
  rawId: ArrayBuffer;
  response: { clientDataJSON: ArrayBuffer; authenticatorData: ArrayBuffer; signature: ArrayBuffer };
}

// registration is what the server takes of a new passkey: its ID, the browser's and the
// authenticator's data, and the public key as SPKI with its algorithm.
export function registration(cred: MadeCredential) {
  const r = cred.response;
  const key = r.getPublicKey?.() ?? null;
  const data = r.getAuthenticatorData?.();
  const alg = r.getPublicKeyAlgorithm?.();
  if (!key || !data || alg === undefined) throw new OldBrowser("the browser does not hand over the passkey's key");
  return { id: toB64url(cred.rawId), clientDataJSON: toB64url(r.clientDataJSON), authenticatorData: toB64url(data), publicKey: toB64url(key), algorithm: alg };
}

// assertion is what the server takes of a check: the signature, and what it is over.
export function assertion(cred: ShownCredential) {
  const r = cred.response;
  return { id: toB64url(cred.rawId), clientDataJSON: toB64url(r.clientDataJSON), authenticatorData: toB64url(r.authenticatorData), signature: toB64url(r.signature) };
}

// passkeysSupported says whether this browser can make and show a passkey here: it has
// WebAuthn and the page is a secure context (the tailnet name over HTTPS).
export function passkeysSupported(env: { PublicKeyCredential?: unknown; credentials?: unknown; isSecureContext?: boolean }): boolean {
  return !!env.PublicKeyCredential && !!env.credentials && env.isSecureContext !== false;
}

// cancelled says the browser's error is the user's no, or the prompt timing out: the
// two WebAuthn does not tell apart, and neither is a fault to report.
export function cancelled(err: unknown): boolean {
  return (err as { name?: unknown } | null)?.name === "NotAllowedError" || (err as { name?: unknown } | null)?.name === "AbortError";
}
