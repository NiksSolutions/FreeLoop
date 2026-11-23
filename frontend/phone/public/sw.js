// The phone app's service worker (GUI-052). It shows a notification when the computer
// sends a Web Push (issue 028) and opens the app when one is tapped. It keeps nothing,
// and answers only a page load of the app that the network fails, with a page saying
// the computer is out of reach (issue 060): the home-screen app showed only white. A
// push holds only a kind and a session's ID, so the text comes from here, in the
// phone's language; it never names the session.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

const text = {
  en: { approval: "A session needs you", done: "A session finished", failed: "A session stopped with an error", budget: "A session reached its budget", other: "Something happened in a session" },
  de: { approval: "Eine Sitzung braucht dich", done: "Eine Sitzung ist fertig", failed: "Eine Sitzung wurde mit einem Fehler beendet", budget: "Eine Sitzung hat ihr Budget erreicht", other: "In einer Sitzung ist etwas passiert" },
  nl: { approval: "Een sessie heeft je nodig", done: "Een sessie is klaar", failed: "Een sessie is met een fout gestopt", budget: "Een sessie heeft haar budget bereikt", other: "Er is iets gebeurd in een sessie" },
};

// The words of the out-of-reach page, as the app's catalogues have them (phone.unreachable).
const offlineText = {
  en: { title: "Can't reach your computer", check: "Check that Tailscale is connected on this phone and on the computer, and that Freeloop is running there.", retry: "Try again" },
  de: { title: "Der Computer ist nicht erreichbar", check: "Prüfen, ob Tailscale auf diesem Telefon und auf dem Computer verbunden ist und Freeloop dort läuft.", retry: "Erneut versuchen" },
  nl: { title: "De computer is niet bereikbaar", check: "Controleren of Tailscale op deze telefoon en op de computer verbonden is en Freeloop daar draait.", retry: "Opnieuw proberen" },
};

// offlinePage is the page a failed load of the app gets: the words, a link to try again
// and the app's colours, with no script.
function offlinePage(language) {
  const w = offlineText[String(language || "en").slice(0, 2).toLowerCase()] || offlineText.en;
  const html =
    '<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">' +
    '<meta name="color-scheme" content="light dark"><title>Freeloop</title><style>' +
    "body{margin:0;padding:calc(env(safe-area-inset-top) + 48px) 24px 24px;font:16px/1.5 -apple-system,system-ui,sans-serif;background:#fff;color:#1d1f23}" +
    "h1{font-size:20px;margin:0 0 12px}p{margin:0 0 24px;color:#5f636b}" +
    "a{display:inline-block;padding:12px 20px;border-radius:22px;background:#2563eb;color:#fff;text-decoration:none}" +
    "@media (prefers-color-scheme:dark){body{background:#1e1e2e;color:#cdd6f4}p{color:#bac2de}a{background:#89b4fa;color:#11111b}}" +
    "</style></head><body><h1>" + w.title + "</h1><p>" + w.check + '</p><a href="/phone/">' + w.retry + "</a></body></html>";
  return new Response(html, { status: 503, headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" } });
}

// Only a page load of the app is answered, and only when the network fails; the app's
// files and every call to the computer go to the network untouched.
self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.mode !== "navigate" || req.method !== "GET" || !new URL(req.url).pathname.startsWith("/phone/")) return;
  event.respondWith(fetch(req).catch(() => offlinePage(self.navigator && self.navigator.language)));
});

// notification is what to show for a push's data: a title from its kind and a link
// to its session. A payload that is missing or not what was expected still gives a
// notification, since a browser may refuse a worker that shows none.
function notification(data, language) {
  const words = text[String(language || "en").slice(0, 2).toLowerCase()] || text.en;
  const kind = data && typeof data.kind === "string" && Object.hasOwn(words, data.kind) ? data.kind : "other";
  const id = data && typeof data.sessionId === "string" && /^[A-Za-z0-9_-]{1,64}$/.test(data.sessionId) ? data.sessionId : "";
  return {
    title: "Freeloop",
    options: {
      body: words[kind],
      icon: "/phone/icon-192.png",
      badge: "/phone/icon-192.png",
      tag: id ? kind + ":" + id : kind,
      data: { url: "/phone/" + (id ? "#session=" + id : "") },
    },
  };
}
self.freeloopNotification = notification;

self.addEventListener("push", (event) => {
  let data = null;
  try {
    data = event.data ? event.data.json() : null;
  } catch (e) {
    data = null;
  }
  const n = notification(data, self.navigator && self.navigator.language);
  event.waitUntil(self.registration.showNotification(n.title, n.options));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || "/phone/";
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((open) => {
      const there = open.find((c) => c.url.includes("/phone/"));
      if (there && there.navigate) return there.navigate(url).then((c) => c && c.focus());
      return self.clients.openWindow(url);
    }),
  );
});
