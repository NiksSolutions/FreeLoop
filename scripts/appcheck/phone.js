// The phone app (GUI-052) at an iPhone's width, 390 px, on the session phonesession.ts
// writes (issue 057), with a phone paired by a key put in the page's storage before it
// loads (APPCHECK_INIT). Helpers are in lib.js.
const snap = (name) => window.webkit.messageHandlers.snap?.postMessage(name)?.catch(() => {});
const rgb = (hex) => `rgb(${parseInt(hex.slice(1, 3), 16)}, ${parseInt(hex.slice(3, 5), 16)}, ${parseInt(hex.slice(5, 7), 16)})`;
const box = (el) => el.getBoundingClientRect();
// lines is how many lines an element's text takes.
const lines = (el) => {
  const r = document.createRange();
  r.selectNodeContents(el);
  return new Set([...r.getClientRects()].filter((q) => q.width > 0).map((q) => Math.round(q.top))).size;
};

await until("the tabs", () => withText(".phone-tabs button", "Sessions"));
withText(".phone-tabs button", "Sessions").click();
(await until("the session", () => withText(".phone-session", "Check phone session"))).click();
await until("the conversation", () => all(".phone-agent").length >= 3 && all(".phone-tool").length >= 3);
await sleep(300);

await step("nothing is wider than the phone", async () => {
  const width = document.documentElement.clientWidth;
  check("the page does not scroll sideways", document.scrollingElement.scrollWidth <= width, `${document.scrollingElement.scrollWidth} px against ${width}`);
  // What scrolls inside its own box may be wider; the box may not.
  const inScroller = (el) => !!el.parentElement?.closest(".md-table, .md-code pre");
  const wide = all(".phone-session-screen *").filter((el) => !inScroller(el) && box(el).right > width + 1);
  check("no element reaches past the screen's edge", wide.length === 0, wide.slice(0, 5).map((el) => `${el.tagName.toLowerCase()}.${el.className} to ${Math.round(box(el).right)} px`).join(", "));
  // Nor does text spill out of its box: a hash in a notice did, on a real iPhone, and the
  // page clipping what spills sideways must not hide it here.
  const spill = all(".phone-session-screen *").filter((el) => !el.closest(".md-table, .md-code pre") && getComputedStyle(el).overflowX === "visible" && el.scrollWidth > el.clientWidth + 1 && el.clientWidth > 0);
  check("no text spills out of its box", spill.length === 0, spill.slice(0, 5).map((el) => `${el.tagName.toLowerCase()}.${el.className} ${el.scrollWidth} in ${el.clientWidth}`).join(", "));
  const table = document.querySelector(".phone-agent .md-table");
  check("a wide table scrolls in its own box", table && table.scrollWidth > table.clientWidth && getComputedStyle(table).overflowX === "auto", table && `${table.scrollWidth} in ${table.clientWidth}`);
  const broken = all(".phone-agent .md-table th, .phone-agent .md-table td").filter((c) => !/\s/.test(c.textContent.trim()) && lines(c) > 1);
  check("no table cell breaks a word", broken.length === 0, broken.map((c) => c.textContent).join(", "));
  const pre = document.querySelector(".phone-agent .md-code pre");
  check("a long code line scrolls in its own box", pre && getComputedStyle(pre).whiteSpace === "pre" && pre.scrollWidth > pre.clientWidth && getComputedStyle(pre).overflowX === "auto", pre && `${getComputedStyle(pre).whiteSpace}, ${pre.scrollWidth} in ${pre.clientWidth}`);
  const copy = document.querySelector(".phone-agent .md-code-bar button");
  check("a code block's Copy button fits its bar", copy && box(copy).height <= 34, copy && `${Math.round(box(copy).height)} px tall`);
});

await step("a tool call is one line, its mark beside its title", async () => {
  const tools = all(".phone-tool");
  const off = tools.filter((t) => {
    const mark = t.querySelector(".phone-tool-mark");
    const title = t.querySelector(".phone-tool-title");
    return !mark || !title || Math.abs(box(mark).top + box(mark).height / 2 - (box(title).top + box(title).height / 2)) > 4 || lines(title) > 1;
  });
  check("each mark stands beside its title, on one line", tools.length >= 3 && off.length === 0, off.map((t) => t.textContent.slice(0, 40)).join(" | "));
  const long = tools.find((t) => t.textContent.includes("Application Support"));
  long.querySelector("summary").click();
  await sleep(100);
  const full = long.querySelector(".phone-tool-full");
  check("tapping a tool shows its whole title", long.open && full && full.textContent.includes("limit_test.go") && full.scrollWidth <= full.clientWidth + 1, full?.textContent);
  long.querySelector("summary").click();
});

await step("messages read as on the desktop", async () => {
  const agent = all(".phone-agent").find((m) => m.textContent.trim() === "Will do.");
  const user = all(".phone-user").find((m) => m.textContent.includes("Push it when CI is green"));
  check("an agent message is text, not a bubble", agent && getComputedStyle(agent).backgroundColor === "rgba(0, 0, 0, 0)", agent && getComputedStyle(agent).backgroundColor);
  const screen = box(document.querySelector(".phone-conversation"));
  check("a user message is a framed bubble on the right", user && parseFloat(getComputedStyle(user).borderTopWidth) > 0 && screen.right - box(user).right < 24 && box(user).left - screen.left > 40, user && `${Math.round(box(user).left)} to ${Math.round(box(user).right)}`);
  const text = getComputedStyle(document.body).color;
  check("the text is in the desktop's colour", [rgb("#1d1f23"), rgb("#cdd6f4")].includes(text), text);
  const p = agent?.querySelector("p");
  check("a one-line reply has no extra space around it", p && getComputedStyle(p).marginTop === "0px" && getComputedStyle(p).marginBottom === "0px", p && `${getComputedStyle(p).marginTop} and ${getComputedStyle(p).marginBottom}`);
  const code = all(".phone-agent .md p code")[0];
  check("inline code is marked", code && getComputedStyle(code).backgroundColor !== "rgba(0, 0, 0, 0)" && getComputedStyle(code).fontFamily.includes("mono"), code && getComputedStyle(code).fontFamily);
});

await step("the message box is one row and grows with what is typed", async () => {
  const composer = document.querySelector(".phone-composer");
  const field = composer.querySelector("textarea");
  const send = withText(".phone-composer button", "Send");
  check("the message box takes little of the screen", box(composer).height <= 90, `${Math.round(box(composer).height)} px of ${innerHeight}`);
  check("Send stands beside the box", send && box(send).top < box(field).bottom && box(send).left > box(field).left, send && `${Math.round(box(send).left)}, ${Math.round(box(send).top)}`);
  const before = box(field).height;
  type(field, "one\ntwo\nthree\nfour");
  await sleep(100);
  check("it grows as lines are typed", box(field).height > before + 20, `${Math.round(before)} to ${Math.round(box(field).height)} px`);
  type(field, "");
  await sleep(100);
  check("and shrinks again when emptied", Math.abs(box(field).height - before) < 2, `${Math.round(box(field).height)} px`);
});

// Issue 059: on a real iPhone the newest message slipped under the message box when the
// keyboard opened. The keyboard shrinks what the page can show; the window shrinking
// does the same here.
await step("the newest message stays in view while the reader is at the end", async () => {
  const resize = (h) => window.webkit.messageHandlers.resize.postMessage([innerWidth, h]);
  const composer = document.querySelector(".phone-composer");
  const field = composer.querySelector("textarea");
  const newest = () => all(".phone-conversation > *").filter((el) => box(el).height > 0).at(-1);
  const shows = () => box(newest()).bottom <= box(composer).top + 2;
  const where = () => `newest ends at ${Math.round(box(newest()).bottom)} px, the message box starts at ${Math.round(box(composer).top)}`;
  const tall = innerHeight;
  window.scrollTo(0, document.scrollingElement.scrollHeight);
  await sleep(300);
  check("at the end, the newest message shows above the message box", shows(), where());
  await resize(tall - 300);
  await sleep(400);
  check("the keyboard opening keeps it in view", shows(), where());
  type(field, "one\ntwo\nthree\nfour");
  await sleep(300);
  check("so does the message box growing", shows(), where());
  type(field, "");
  await resize(tall);
  await sleep(400);
  check("and the keyboard closing", shows(), where());
  // Scrolled up to read, the reader stays where they are.
  window.scrollTo(0, document.scrollingElement.scrollHeight - innerHeight - 400);
  await sleep(300);
  const y = window.scrollY;
  await resize(tall - 300);
  await sleep(400);
  check("scrolled up, the keyboard opening leaves the view where the reader put it", Math.abs(window.scrollY - y) < 40 && !shows(), `${Math.round(y)} to ${Math.round(window.scrollY)} px`);
  await resize(tall);
  await sleep(400);
});

// Issue 060: with the keyboard open, an empty band stood between the message box and
// the keyboard: the room kept for the home bar, which the keyboard covers. A Mac keeps
// no such room, so the check gives the page an iPhone's.
await step("typing, the message box sits on the keyboard", async () => {
  const composer = document.querySelector(".phone-composer");
  const field = composer.querySelector("textarea");
  const room = () => parseFloat(getComputedStyle(composer).paddingBottom);
  document.documentElement.style.setProperty("--safe-bottom", "34px");
  await sleep(100);
  check("not typing, it keeps the home bar's room", room() >= 40, `${room()} px`);
  field.focus();
  await sleep(100);
  check("typing, that room goes, as the keyboard covers the home bar", room() <= 10, `${room()} px`);
  field.blur();
  await sleep(100);
  check("and comes back when typing ends", room() >= 40, `${room()} px`);
  document.documentElement.style.removeProperty("--safe-bottom");
});

window.scrollTo(0, 0);
await sleep(100);
await snap("phone-conversation");

await step("pictures go from the phone", async () => {
  // A picture as the phone's photo picker or a pasted screenshot gives one: a PNG file.
  const canvas = document.createElement("canvas");
  canvas.width = 60;
  canvas.height = 40;
  canvas.getContext("2d").fillRect(0, 0, 60, 40);
  const png = await new Promise((resolve) => canvas.toBlob(resolve, "image/png"));
  const files = (name) => {
    const dt = new DataTransfer();
    dt.items.add(new File([png], name, { type: "image/png" }));
    return dt;
  };
  check("the message box has a picture button", !!named(".phone-composer button", "Add a picture"));
  const input = document.querySelector('.phone-composer input[type="file"]');
  check("it opens the phone's photo picker", input && input.accept === "image/*" && input.multiple);
  input.files = files("shot.png").files;
  input.dispatchEvent(new Event("change", { bubbles: true }));
  const thumbs = () => all(".phone-pictures img");
  await until("the thumbnail", () => thumbs().length === 1, 5000);
  check("a chosen picture shows as a thumbnail", thumbs()[0]?.src.startsWith("data:image/png;base64,"));
  const field = document.querySelector(".phone-composer textarea");
  const idle = box(field).height;
  field.dispatchEvent(new ClipboardEvent("paste", { clipboardData: files("pasted.png"), bubbles: true, cancelable: true }));
  await until("the pasted picture", () => thumbs().length === 2, 5000).catch(() => null);
  check("a pasted picture is attached too", thumbs().length === 2, `${thumbs().length} thumbnails`);
  named(".phone-pictures button", `Remove picture ${thumbs().length}`)?.click();
  await until("one thumbnail", () => thumbs().length === 1, 3000).catch(() => null);
  check("a picture can be removed before sending", thumbs().length === 1);
  const send = withText(".phone-composer button", "Send");
  check("a picture alone can be sent", send && !send.disabled);
  send.click();
  await until("sent", () => !document.querySelector(".phone-pictures"), 10000);
  const sent = await until("the message's picture", () => all(".phone-user .phone-sent-pictures img").find((i) => i.complete && i.naturalWidth > 0), 10000).catch(() => null);
  check("the sent message shows its picture, read back with the phone's key", !!sent);
  check("and says it came from the phone", sent?.closest(".phone-user").textContent.includes("From the phone (Check iPhone)"), sent?.closest(".phone-user").textContent);
  // The demo agent works on it, and asks before it writes.
  const stop = await until("Stop", () => named(".phone-composer button", "Stop"), 10000).catch(() => null);
  check("while the agent works, Stop stands beside a box still one row high", stop && Math.abs(box(field).height - idle) < 2 && box(stop).top < box(field).bottom, `${Math.round(box(field).height)} px against ${Math.round(idle)}`);
  sent?.scrollIntoView({ block: "center" });
  await sleep(100);
  await snap("phone-picture");
});
return results;
