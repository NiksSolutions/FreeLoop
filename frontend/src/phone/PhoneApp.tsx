import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { language, LANGUAGES, setLanguage, t, type Language } from "../i18n.ts";
import { Markdown } from "../Markdown.tsx";
import { createBatcher } from "../eventBatch.ts";
import { apply, viewOf, emptyTranscript, phoneName, transcriptFrom, type Item, type Transcript, type TranscriptEvent } from "../transcript.ts";
import { MAX_IMAGES, room } from "../images.ts";
import { prepareImage } from "../imageprep.ts";
import { api, followSession, followSummary, Unpaired, type Picture, type PhoneRequest, type Summary } from "./api.ts";
import { applicationServerKey, deviceName, onceOptions, pairCode, pastedPictures, pushBody, sessionFromHash, pushSupported, toolCommand } from "./phone.ts";

// The phone app (GUI-052): what needs the user, the sessions, one session live with
// a message box, and settings. From here a plain request is allowed or denied once;
// anything more is answered on the computer.

type Screen = { kind: "needs" } | { kind: "sessions" } | { kind: "session"; id: string } | { kind: "settings" };

export default function PhoneApp() {
  const [paired, setPaired] = useState<boolean | null>(null);
  const [summary, setSummary] = useState<Summary | null>(null);
  // A tapped notification opens /phone/#session=<id> (issue 029), at start or while the app is open.
  const [screen, setScreen] = useState<Screen>(() => {
    const id = sessionFromHash(location.hash);
    return id ? { kind: "session", id } : { kind: "needs" };
  });
  const [, setLang] = useState<Language>(language());

  useEffect(() => {
    const open = () => {
      const id = sessionFromHash(location.hash);
      if (id) setScreen({ kind: "session", id });
    };
    window.addEventListener("hashchange", open);
    return () => window.removeEventListener("hashchange", open);
  }, []);

  // Paired once the summary answers; the stream keeps it fresh.
  useEffect(() => {
    if (paired === false) return;
    let stop = () => {};
    api
      .summary()
      .then((s) => {
        setPaired(true);
        setSummary(s);
        stop = followSummary(setSummary, () =>
          api.summary().catch((err) => {
            if (err instanceof Unpaired) setPaired(false);
          }),
        );
      })
      .catch((err) => setPaired(err instanceof Unpaired ? false : true));
    return () => stop();
  }, [paired]);

  // While the app is open its icon shows how many requests wait, from the summary the
  // authenticated stream keeps fresh (never from a push).
  const waiting = summary?.attention.reduce((n, a) => n + a.requests.length + (a.error ? 1 : 0), 0) ?? 0;
  useEffect(() => {
    const nav = navigator as Navigator & { setAppBadge?: (n: number) => Promise<void>; clearAppBadge?: () => Promise<void> };
    if (waiting > 0) nav.setAppBadge?.(waiting).catch(() => {});
    else nav.clearAppBadge?.().catch(() => {});
  }, [waiting]);

  if (paired === null) return <p className="phone-note">{t("phone.loading")}</p>;
  if (!paired) return <Pairing onPaired={() => setPaired(true)} />;

  const title =(sid: string) => summary?.sessions.find((s) => s.id === sid)?.title ?? sid;
  return (
    <div className="phone">
      {screen.kind === "session" ? (
        <SessionScreen id={screen.id} title={title(screen.id)} onBack={() => setScreen({ kind: "sessions" })} />
      ) : (
        <>
          <header className="phone-bar">
            <h1>Freeloop</h1>
          </header>
          <main className="phone-main">
            {screen.kind === "needs" && <Needs summary={summary} title={title} onOpen={(id) => setScreen({ kind: "session", id })} />}
            {screen.kind === "sessions" && <Sessions summary={summary} onOpen={(id) => setScreen({ kind: "session", id })} />}
            {screen.kind === "settings" && (
              <Settings
                onLanguage={(l) => {
                  setLanguage(l, navigator.languages);
                  localStorage.setItem("freeloop.phone.language", l);
                  setLang(l);
                }}
                onSignedOut={() => setPaired(false)}
              />
            )}
          </main>
          <nav className="phone-tabs" aria-label={t("phone.tabs")}>
            <button aria-current={screen.kind === "needs" ? "page" : undefined} onClick={() => setScreen({ kind: "needs" })}>
              {t("phone.needs")}
              {waiting > 0 && <span className="phone-count">{waiting}</span>}
            </button>
            <button aria-current={screen.kind === "sessions" ? "page" : undefined} onClick={() => setScreen({ kind: "sessions" })}>
              {t("phone.sessions")}
            </button>
            <button aria-current={screen.kind === "settings" ? "page" : undefined} onClick={() => setScreen({ kind: "settings" })}>
              {t("phone.settings")}
            </button>
          </nav>
        </>
      )}
    </div>
  );
}

function Pairing({ onPaired }: { onPaired: () => void }) {
  const [code, setCode] = useState(pairCode(location.hash));
  const [name, setName] = useState(deviceName(navigator.userAgent));
  const [state, setState] = useState<"idle" | "waiting" | "refused">("idle");
  const [error, setError] = useState("");
  const pair = () => {
    setError("");
    api
      .pair(code, name)
      .then(({ id }) => {
        setState("waiting");
        const ask = () =>
          api.pairAnswer(id).then(({ answer }) => {
            if (answer === "allowed") {
              history.replaceState(null, "", location.pathname);
              onPaired();
            } else if (answer === "refused") setState("refused");
            else setTimeout(ask, 1000);
          });
        ask().catch((err) => setError(String(err.message ?? err)));
      })
      .catch((err) => setError(String(err.message ?? err)));
  };
  return (
    <main className="phone-pair">
      <h1>{t("phone.pair.title")}</h1>
      <p>{t("phone.pair.how")}</p>
      {state === "waiting" ? (
        <p role="status" className="phone-waiting">
          {t("phone.pair.confirm")}
        </p>
      ) : (
        <>
          <label>
            {t("phone.pair.code")}
            <input value={code} onChange={(e) => setCode(e.target.value)} autoCapitalize="characters" autoComplete="off" spellCheck={false} />
          </label>
          <label>
            {t("phone.pair.name")}
            <input value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <button className="primary" onClick={pair} disabled={!code.trim()}>
            {t("phone.pair.button")}
          </button>
        </>
      )}
      {state === "refused" && <p role="alert">{t("phone.pair.refused")}</p>}
      {error && <p role="alert">{error}</p>}
    </main>
  );
}

function RequestCard({ req, onDone }: { req: PhoneRequest; onDone?: () => void }) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const { allow, deny } = onceOptions(req.options);
  const answer = (optionId: string) => {
    setBusy(true);
    setError("");
    api
      .decide(req.requestId, optionId)
      .then(() => onDone?.())
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setBusy(false));
  };
  return (
    <div className="phone-request">
      <strong>{req.title || req.tool}</strong>
      {req.command && <pre>{req.command}</pre>}
      {(req.paths ?? []).length > 0 && <pre>{req.paths!.join("\n")}</pre>}
      {req.leavesSandbox && <p className="phone-warn">{t("phone.request.leavesSandbox")}</p>}
      {req.onLaptop || !allow ? (
        <p className="phone-note">{t("phone.request.onLaptop")}</p>
      ) : (
        <div className="phone-row">
          <button className="primary" disabled={busy} onClick={() => answer(allow.optionId)}>
            {t("phone.request.allowOnce")}
          </button>
          {deny && (
            <button disabled={busy} onClick={() => answer(deny.optionId)}>
              {t("phone.request.deny")}
            </button>
          )}
        </div>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}

function Needs({ summary, title, onOpen }: { summary: Summary | null; title: (sid: string) => string; onOpen: (id: string) => void }) {
  const list = summary?.attention ?? [];
  if (list.length === 0) return <p className="phone-note">{t("phone.needs.none")}</p>;
  return (
    <ul className="phone-list">
      {list.map((a) => (
        <li key={a.sessionId}>
          <button className="phone-link" onClick={() => onOpen(a.sessionId)}>
            {title(a.sessionId)}
          </button>
          {a.requests.map((r) => (
            <RequestCard key={r.requestId} req={r} />
          ))}
          {a.error && <p className="phone-warn">{a.error}</p>}
          {a.stuck && <p className="phone-warn">{a.stuck}</p>}
        </li>
      ))}
    </ul>
  );
}

const statusKey = { running: "phone.status.running", waiting_permission: "phone.status.waiting", waiting_input: "phone.status.waiting", error: "phone.status.error", queued: "phone.status.queued" } as const;

function Sessions({ summary, onOpen }: { summary: Summary | null; onOpen: (id: string) => void }) {
  const list = summary?.sessions ?? [];
  if (list.length === 0) return <p className="phone-note">{t("phone.sessions.none")}</p>;
  return (
    <ul className="phone-list">
      {list.map((s) => (
        <li key={s.id}>
          <button className="phone-session" onClick={() => onOpen(s.id)}>
            <span>{s.title}</span>
            <span className="phone-muted">
              {s.projectName}
              {s.status in statusKey && ` · ${t(statusKey[s.status as keyof typeof statusKey])}`}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}

// How many events a session opens with on the phone: its end, as on the computer (issue 046).
const PHONE_TAIL = 2000;

function SessionScreen({ id, title, onBack }: { id: string; title: string; onBack: () => void }) {
  const [tr, setTr] = useState<Transcript>(emptyTranscript);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const end = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setTr(emptyTranscript);
    // One update per frame, however many events arrive (NFR-PERF-001).
    // The stream opens at the session's end, so the first events say where it starts.
    const batcher = createBatcher<TranscriptEvent>((batch) =>
      setTr((cur) => apply(cur.length === 0 && batch.length > 0 ? transcriptFrom(batch.reduce((m, ev) => Math.min(m, ev.seq), Infinity)) : cur, batch).transcript),
    );
    const stop = followSession(id, 0, (ev) => batcher.push(ev), () => {}, PHONE_TAIL);
    return () => {
      stop();
      batcher.cancel();
    };
  }, [id]);
  const view = useMemo(() => viewOf(tr), [tr]);
  useEffect(() => end.current?.scrollIntoView({ block: "end" }), [tr.lastSeq]);
  // The message box grows with what is typed, up to its CSS max-height, and shrinks back.
  const field = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = field.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight + el.offsetHeight - el.clientHeight}px`;
  }, [text]);
  // Pictures for the next message (issue 058), scaled down as on the computer.
  const [pictures, setPictures] = useState<(Picture & { id: number })[]>([]);
  const [sending, setSending] = useState(false);
  const chooser = useRef<HTMLInputElement>(null);
  const nextPicture = useRef(0);
  const attach = (files: Blob[]) => {
    const take = files.slice(0, room(pictures.length, files.length));
    setError(take.length < files.length ? t("attach.tooManyPictures", { max: MAX_IMAGES }) : "");
    for (const file of take)
      prepareImage(file)
        .then(({ mediaType, data, url }) => {
          // Shown from its data, which the page's policy allows, not from the object URL.
          URL.revokeObjectURL(url);
          setPictures((cur) => (cur.length < MAX_IMAGES ? [...cur, { mediaType, data, id: nextPicture.current++ }] : cur));
        })
        .catch((err) => setError(String(err.message ?? err)));
  };
  const send = () => {
    const msg = text.trim();
    if (!msg && pictures.length === 0) return;
    setError("");
    setSending(true);
    api
      .prompt(
        id,
        msg,
        pictures.map(({ mediaType, data }) => ({ mediaType, data })),
      )
      .then(() => {
        setText("");
        setPictures([]);
      })
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setSending(false));
  };
  return (
    <div className="phone-session-screen">
      <header className="phone-bar">
        <button className="phone-back" onClick={onBack} aria-label={t("phone.back")}>
          ‹
        </button>
        <h1>{title}</h1>
      </header>
      <main className="phone-conversation">
        {tr.firstSeq > 1 && <p className="phone-muted">{t("phone.session.earlier")}</p>}
        {view.items.map((item) => (
          <PhoneItem key={item.key} item={item} sid={id} />
        ))}
        <div ref={end} />
      </main>
      <footer className="phone-composer">
        {error && <p role="alert">{error}</p>}
        {pictures.length > 0 && (
          <ul className="phone-pictures" aria-label={t("composer.pictures")}>
            {pictures.map((p, i) => (
              <li key={p.id}>
                <img src={`data:${p.mediaType};base64,${p.data}`} alt={t("composer.picture", { n: i + 1 })} />
                <button className="phone-picture-remove" aria-label={t("composer.removePicture", { n: i + 1 })} onClick={() => setPictures((cur) => cur.filter((q) => q.id !== p.id))}>
                  ×
                </button>
              </li>
            ))}
          </ul>
        )}
        <div className="phone-compose-row">
          <button className="phone-add-picture" aria-label={t("phone.composer.addPicture")} onClick={() => chooser.current?.click()}>
            <svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="5" width="18" height="14" rx="2" />
              <circle cx="9" cy="10" r="1.6" />
              <path d="m21 16-5-5-8 8" />
            </svg>
          </button>
          {/* The phone's photo picker or camera; a pasted screenshot is attached as well. */}
          <input
            ref={chooser}
            type="file"
            accept="image/*"
            multiple
            hidden
            onChange={(e) => {
              attach([...(e.target.files ?? [])]);
              e.target.value = "";
            }}
          />
          <textarea
            ref={field}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onPaste={(e) => {
              const pasted = pastedPictures(e.clipboardData.files);
              if (pasted.length === 0) return;
              e.preventDefault();
              attach(pasted);
            }}
            placeholder={t("phone.composer.placeholder")}
            aria-label={t("phone.composer.label")}
            rows={1}
          />
          {view.running && (
            <button className="phone-stop" aria-label={t("phone.composer.stop")} onClick={() => api.cancel(id).catch((err) => setError(String(err.message ?? err)))}>
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <rect x="7" y="7" width="10" height="10" rx="1.5" fill="currentColor" />
              </svg>
            </button>
          )}
          <button className="primary" onClick={send} disabled={sending || (!text.trim() && pictures.length === 0)}>
            {view.running ? t("phone.composer.queue") : t("phone.composer.send")}
          </button>
        </div>
      </footer>
    </div>
  );
}

// The pictures a message carried (issue 058), read with the phone's key once and kept
// while the app is open: data URLs, or "" for one that could not be read. They are the
// user's own attachments; nothing a model writes is ever shown as a picture.
const sentPictures = new Map<string, string>();

function SentPictures({ sid, images }: { sid: string; images: { name: string }[] }) {
  const [, setLoaded] = useState(0);
  useEffect(() => {
    let live = true;
    for (const img of images) {
      const key = `${sid}/${img.name}`;
      if (sentPictures.has(key)) continue;
      api
        .attachment(sid, img.name)
        .then((a) => sentPictures.set(key, a.mediaType.startsWith("image/") ? `data:${a.mediaType};base64,${a.data}` : ""))
        .catch(() => sentPictures.set(key, ""))
        .finally(() => live && setLoaded((n) => n + 1));
    }
    return () => {
      live = false;
    };
  }, [sid, images]);
  return (
    <div className="phone-sent-pictures">
      {images.map((img, i) => {
        const src = sentPictures.get(`${sid}/${img.name}`);
        if (src === "") return <span key={img.name} className="phone-muted">{t("transcript.picture.missing")}</span>;
        if (!src) return <span key={img.name} className="phone-picture-loading" role="img" aria-label={t("transcript.picture.loading")} />;
        return <img key={img.name} src={src} alt={t("transcript.picture.alt", { number: i + 1 })} />;
      })}
    </div>
  );
}

function PhoneItem({ item, sid }: { item: Item; sid: string }) {
  switch (item.kind) {
    case "user":
      return (
        <div className="phone-msg phone-user">
          {(item.images ?? []).length > 0 && <SentPictures sid={sid} images={item.images!} />}
          {item.text}
          {phoneName(item.via) && <div className="phone-muted">{t("session.item.fromPhone", { name: phoneName(item.via) })}</div>}
        </div>
      );
    case "agent":
      return (
        <div className="phone-msg phone-agent">
          <Markdown text={item.text} />
        </div>
      );
    case "tool": {
      const p = item.permission;
      const waiting = p && !p.decided;
      const command = toolCommand(item.input);
      const [mark, state] = item.status === "failed" ? ["✗", t("phone.tool.failed")] : item.status === "completed" ? ["✓", t("phone.tool.done")] : ["…", t("phone.tool.running")];
      // One line; tapping it shows the whole title and the command (issue 057).
      const call = (
        <details className={`phone-tool phone-tool-${item.status}`}>
          <summary>
            <span className="phone-tool-mark" role="img" aria-label={state}>
              {mark}
            </span>
            <span className="phone-tool-title">{item.title}</span>
          </summary>
          <div className="phone-tool-full">
            <span>{item.title}</span>
            {command && command !== item.title && <pre>{command}</pre>}
          </div>
        </details>
      );
      if (!waiting) return call;
      return (
        <div className="phone-call">
          {call}
          <RequestCard req={{ requestId: p.requestId, sessionId: "", toolCallId: item.toolCallId, tool: item.tool, command: p.command, paths: p.paths, options: p.options, onLaptop: !!p.form }} />
        </div>
      );
    }
    case "error":
      return <p className="phone-warn">{item.message}</p>;
    case "notice":
      return <p className="phone-muted">{item.message}</p>;
    default:
      return null;
  }
}

function Settings({ onLanguage, onSignedOut }: { onLanguage: (l: Language) => void; onSignedOut: () => void }) {
  const [error, setError] = useState("");
  return (
    <div className="phone-settings">
      <label>
        {t("phone.settings.language")}
        <select value={language()} onChange={(e) => onLanguage(e.target.value as Language)}>
          {LANGUAGES.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name}
            </option>
          ))}
        </select>
      </label>
      <Notify />
      <p className="phone-note">{t("phone.settings.about")}</p>
      <p className="phone-note">{t("phone.settings.install")}</p>
      <button
        onClick={() =>
          api
            .logout()
            .then(onSignedOut)
            .catch((err) => setError(String(err.message ?? err)))
        }
      >
        {t("phone.settings.signOut")}
      </button>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}

// Notify is the phone's switch for push: on a tap it asks the browser's permission,
// subscribes with the computer's key and hands the subscription to the computer; off
// takes it back. The browser's own subscription is the truth about whether it is on.
function Notify() {
  const supported = pushSupported({
    serviceWorker: "serviceWorker" in navigator ? navigator.serviceWorker : undefined,
    PushManager: "PushManager" in window ? PushManager : undefined,
    Notification: "Notification" in window ? Notification : undefined,
  });
  const [on, setOn] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!supported) return;
    navigator.serviceWorker.ready
      .then((reg) => reg.pushManager.getSubscription())
      .then((sub) => setOn(sub !== null))
      .catch(() => {});
  }, [supported]);
  if (!supported) return <p className="phone-note">{t("phone.settings.notify.unsupported")}</p>;
  const turnOn = async () => {
    if ((await Notification.requestPermission()) !== "granted") throw new Error(t("phone.settings.notify.denied"));
    const reg = await navigator.serviceWorker.ready;
    const { key } = await api.pushKey();
    const sub = (await reg.pushManager.getSubscription()) ?? (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: applicationServerKey(key) }));
    try {
      await api.pushSubscribe(pushBody(sub));
    } catch (err) {
      await sub.unsubscribe().catch(() => {});
      throw err;
    }
    setOn(true);
  };
  const turnOff = async () => {
    const reg = await navigator.serviceWorker.ready;
    const sub = await reg.pushManager.getSubscription();
    await api.pushUnsubscribe();
    await sub?.unsubscribe();
    setOn(false);
  };
  return (
    <>
      <button
        disabled={busy}
        onClick={() => {
          setBusy(true);
          setError("");
          (on ? turnOff() : turnOn()).catch((err) => setError(String(err.message ?? err))).finally(() => setBusy(false));
        }}
      >
        {on ? t("phone.settings.notify.on") : t("phone.settings.notify")}
      </button>
      <p className="phone-note">{t("phone.settings.notify.hint")}</p>
      {error && <p role="alert">{error}</p>}
    </>
  );
}
