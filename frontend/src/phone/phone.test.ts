import { test } from "node:test";
import assert from "node:assert/strict";
import { applicationServerKey, sessionFromHash, deviceName, pushBody, onceOptions, pairCode, parseSSE, pickLanguage, pushSupported, reconnectDelay, toolCommand, pastedPictures, whileShown } from "./phone.ts";
import { follow } from "./api.ts";

test("a tapped notification's link opens that session, and nothing else does", () => {
  assert.equal(sessionFromHash("#session=ses_01-A"), "ses_01-A");
  assert.equal(sessionFromHash("#pair=ABC&session=x1"), "x1");
  assert.equal(sessionFromHash(""), "");
  assert.equal(sessionFromHash("#session="), "");
  assert.equal(sessionFromHash("#session=../../x"), "");
  assert.equal(sessionFromHash("#session=<b>"), "");
  assert.equal(sessionFromHash("#session=" + "a".repeat(65)), "");
});

test("the phone answers only once", () => {
  const options = [
    { optionId: "a1", name: "Allow", kind: "allow_once" },
    { optionId: "a2", name: "Always", kind: "allow_always" },
    { optionId: "r1", name: "Deny", kind: "reject_once" },
  ];
  assert.deepEqual(onceOptions(options), { allow: options[0], deny: options[2] });
  assert.deepEqual(onceOptions([{ optionId: "x", name: "Always", kind: "allow_always" }]), { allow: undefined, deny: undefined });
  assert.deepEqual(onceOptions(null), { allow: undefined, deny: undefined });
});

test("a pairing link carries its code", () => {
  assert.equal(pairCode("#pair=AB12-CD34"), "AB12-CD34");
  assert.equal(pairCode("#x=1&pair=ZZ"), "ZZ");
  assert.equal(pairCode("#other"), "");
  assert.equal(pairCode(""), "");
});

test("a phone's first name and language", () => {
  assert.equal(deviceName("Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X)"), "iPhone");
  assert.equal(deviceName("Mozilla/5.0 (Linux; Android 16)"), "Android phone");
  assert.equal(deviceName("curl"), "Phone");
  assert.equal(pickLanguage(["fr-FR", "nl-BE", "en"]), "nl");
  assert.equal(pickLanguage(["fr"]), "en");
});

test("a server-sent stream is read in whole messages, whatever the chunks", () => {
  const first = parseSSE("", "id: 4\nevent: event\ndata: {\"a\":1}\n\n: ping\n\nevent: summ");
  assert.deepEqual(first.messages, [{ id: "4", event: "event", data: '{"a":1}' }]);
  assert.equal(first.rest, "event: summ");
  const second = parseSSE(first.rest, "ary\ndata: x\ndata: y\n\n");
  assert.deepEqual(second.messages, [{ id: "", event: "summary", data: "x\ny" }]);
  assert.equal(second.rest, "");
  // Carriage returns, as some proxies send, and a message with only a comment.
  assert.deepEqual(parseSSE("", "event: event\r\ndata: z\r\n\r\n").messages, [{ id: "", event: "event", data: "z" }]);
  assert.deepEqual(parseSSE("", ": keep\n\n").messages, []);
  // A message with no event name is the default one.
  assert.deepEqual(parseSSE("", "data: m\n\n").messages, [{ id: "", event: "message", data: "m" }]);
});

test("a stream that dropped goes on after a pause that grows to a limit", () => {
  assert.equal(reconnectDelay(0), 500);
  assert.equal(reconnectDelay(1), 1000);
  assert.equal(reconnectDelay(2), 2000);
  assert.equal(reconnectDelay(20), 15000);
});

test("the push key from the server is read as bytes for subscribe", () => {
  // 0x04 then 64 bytes: a P-256 point, as the server sends it in base64url.
  const bytes = Uint8Array.from([4, ...Array.from({ length: 64 }, (_, i) => 250 - i)]);
  const b64 = Buffer.from(bytes).toString("base64").replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  assert.deepEqual(Array.from(applicationServerKey(b64)), Array.from(bytes));
});

test("a subscription is posted as its endpoint and two keys", () => {
  const sub = { toJSON: () => ({ endpoint: "https://fcm.googleapis.com/x", expirationTime: null, keys: { p256dh: "P", auth: "A" } }) };
  assert.deepEqual(pushBody(sub), { endpoint: "https://fcm.googleapis.com/x", keys: { p256dh: "P", auth: "A" } });
  assert.throws(() => pushBody({ toJSON: () => ({ endpoint: "https://x" }) }));
});

test("push needs a service worker, push and notifications in the browser", () => {
  assert.equal(pushSupported({}), false);
  assert.equal(pushSupported({ serviceWorker: {}, PushManager: {}, Notification: {} }), true);
  assert.equal(pushSupported({ serviceWorker: {}, PushManager: {} }), false);
});

test("a tapped tool call shows the command it ran, if its input names one", () => {
  assert.equal(toolCommand({ command: "go test ./..." }), "go test ./...");
  assert.equal(toolCommand({ path: "a.go" }), "");
  assert.equal(toolCommand({ command: 42 }), "");
  assert.equal(toolCommand(null), "");
  assert.equal(toolCommand(undefined), "");
  assert.equal(toolCommand("go test"), "");
});

test("a pasted screenshot is attached, and pasted text is left to paste as text", () => {
  const files = [{ type: "image/png", name: "shot" }, { type: "text/plain", name: "note" }, { type: "image/heic", name: "photo" }, { type: "", name: "unknown" }];
  assert.deepEqual(pastedPictures(files).map((f) => f.name), ["shot", "photo"]);
  assert.deepEqual(pastedPictures([]), []);
});

// A page as the phone's browser has it: shown or hidden, and word when that changes.
class FakePage extends EventTarget {
  visibilityState: DocumentVisibilityState = "visible";
  set(state: DocumentVisibilityState) {
    this.visibilityState = state;
    this.dispatchEvent(new Event("visibilitychange"));
  }
}

test("what follows the computer runs only while the page is shown", () => {
  const page = new FakePage();
  let opened = 0;
  let closed = 0;
  const stop = whileShown(page, () => {
    opened++;
    return () => closed++;
  });
  assert.deepEqual([opened, closed], [1, 0]);
  // Locked, or another app in front: closed, so the computer pushes instead (issue 076).
  page.set("hidden");
  assert.deepEqual([opened, closed], [1, 1]);
  page.set("hidden");
  assert.deepEqual([opened, closed], [1, 1]);
  page.set("visible");
  page.set("visible");
  assert.deepEqual([opened, closed], [2, 1]);
  stop();
  page.set("hidden");
  page.set("visible");
  assert.deepEqual([opened, closed], [2, 2]);
  // Opened hidden, it waits to be shown.
  const hidden = new FakePage();
  hidden.visibilityState = "hidden";
  let n = 0;
  whileShown(hidden, () => (n++, () => {}));
  assert.equal(n, 0);
  hidden.set("visible");
  assert.equal(n, 1);
});

// A computer that answers a stream with what the test sends it, until the request is aborted.
function fakeComputer() {
  const streams: { headers: Record<string, string>; signal: AbortSignal; send: (text: string) => void }[] = [];
  const fetch = async (_url: string, init: RequestInit) => {
    let push: (text: string) => void = () => {};
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        push = (text) => c.enqueue(new TextEncoder().encode(text));
        init.signal?.addEventListener("abort", () => c.error(new DOMException("aborted", "AbortError")));
      },
    });
    streams.push({ headers: init.headers as Record<string, string>, signal: init.signal!, send: (text) => push(text) });
    return new Response(body, { status: 200 });
  };
  return { streams, fetch };
}
const settle = () => new Promise((r) => setTimeout(r, 20));

test("a hidden page closes its stream without saying the computer is out of reach, and goes on where it was when shown", async () => {
  const computer = fakeComputer();
  const real = { fetch: globalThis.fetch, storage: (globalThis as { localStorage?: unknown }).localStorage };
  globalThis.fetch = computer.fetch as typeof fetch;
  (globalThis as { localStorage?: unknown }).localStorage = { getItem: () => "key", removeItem: () => {} };
  try {
    const page = new FakePage();
    const got: string[] = [];
    let lost = 0;
    const stop = follow("/api/sessions/s/stream", (m) => got.push(m.id), () => lost++, page);
    await settle();
    assert.equal(computer.streams.length, 1);
    assert.equal(computer.streams[0].headers.Authorization, "Bearer key");
    computer.streams[0].send("id: 7\nevent: event\ndata: {}\n\n");
    await settle();
    assert.deepEqual(got, ["7"]);
    page.set("hidden");
    await settle();
    assert.ok(computer.streams[0].signal.aborted, "the stream stayed open while hidden");
    assert.equal(lost, 0, "hiding the page was taken for losing the computer");
    assert.equal(computer.streams.length, 1);
    page.set("visible");
    await settle();
    assert.equal(computer.streams.length, 2);
    assert.equal(computer.streams[1].headers["Last-Event-ID"], "7");
    stop();
    await settle();
    assert.ok(computer.streams[1].signal.aborted);
    assert.equal(lost, 0);
  } finally {
    globalThis.fetch = real.fetch;
    (globalThis as { localStorage?: unknown }).localStorage = real.storage;
  }
});
