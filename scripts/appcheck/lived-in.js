// The second pass of the app check: the same app on a data folder set up the way a
// user's looks after weeks of use, not a fresh one (scripts/appcheck.sh, "lived-in").
// A default profile on an external agent, a global rule allowing shell, dozens of
// skills with long descriptions, a default token optimizer, a hook written by hand, an
// MCP server that cannot start, many sessions. Bugs it was written for passed the fresh
// pass and broke here: the demo ran on the default profile's agent, and the palette
// filled with the agent's skills. Helpers are in lib.js.

await until("the session list", () => all(".session-row").length >= 30, 20000);

await step("a lived-in app opens without errors", async () => {
  await sleep(1500);
  const errors = all(".banner.error").map((b) => b.textContent.trim());
  check("no error banner", errors.length === 0, errors.join(" | "));
  const problems = withText(".banner.warn", "Configuration problems");
  check("the configuration reads without problems", !problems, problems?.textContent);
  check("every session is listed", all(".session-row").length >= 30, `${all(".session-row").length} rows`);
});

await step("a global rule allows any shell command", async () => {
  // Added in the app, as a user does: one written into rules.yaml by hand waits for approval.
  await openPalette("Policy");
  (await until("the entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "Policy"), 4000)).click();
  const effect = await until("the rule form", () => named("select", "Effect"), 8000);
  const choose = (el, value) => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  };
  choose(effect, "allow");
  choose(named("select", "Tool"), "shell");
  await sleep(100);
  withText("button", "Add rule").click();
  await until("the rule", () => all(".rule-list li, ul li").some((li) => /allow\s+shell/i.test(li.textContent)), 5000);
  check("the global rule is in place", true);
});

// The made-up long session (issue 040, 80,000 events). It opens at its end (issue 046):
// the window reads its last events, draws the last few hundred items and reads what came
// before a moment later, a page at a time. The time from a click to the end drawn is
// held to a second, on a slower machine (CI, APPCHECK_SLOW) printed. It is the app's
// own count (the probe's drawn), since the check's timers wait a second each while
// the window is behind others.
const conversationOf = (pane) => pane.querySelector(".conversation");
const firstDrawn = (c) => [...c.children].find((el) => el.matches("[data-item], details.tool-run"));
const keyOf = (el) => el?.getAttribute("data-item") ?? el?.querySelector("[data-item]")?.getAttribute("data-item");
const searchBox = () => document.querySelector('.filter-search input[type="search"]');
const searchFor = (q) => {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(searchBox(), q);
  searchBox().dispatchEvent(new Event("input", { bubbles: true }));
};
const longHits = () => {
  const li = all(".search-session").find((s) => s.textContent.includes("Long session"))?.closest("li");
  const found = li ? all("button.search-hit", li) : [];
  return found.length > 0 && found;
};

await step("the long session opens at its end, in under a second", async () => {
  window.__freeloopCount = { renders: 0, events: 0 };
  const t0 = performance.now();
  openSession("Long session");
  const pane = await until("the long session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Long session" && activePane(), 8000);
  // The end is drawn when a turn's stop line is there and the view sits at the bottom.
  await until("the end of the long session", () => {
    const c = conversationOf(pane);
    return c && all(".turn-stop", pane).length > 0 && toTheEnd(c) < 40;
  }, 120000);
  const ms = Math.round((window.__freeloopCount.drawn ?? performance.now()) - t0);
  delete window.__freeloopCount;
  const drawn = all("[data-item]", pane).length;
  if (slowMachine) check(`the long session's end is drawn after ${ms} ms (a slow machine, so not held to a second)`, true);
  else check(`the long session's end is drawn after ${ms} ms, under a second`, ms < 1000, `${ms} ms`);
  check(`it draws a few hundred items, not all of them (${drawn})`, drawn > 0 && drawn < 1500, `${drawn} items`);
  const first = keyOf(firstDrawn(conversationOf(pane)));
  await until("what came before read", () => !conversationOf(pane).hasAttribute("data-partial"), 120000);
  await sleep(300);
  const c = conversationOf(pane);
  check(`what came before is read after ${Math.round(performance.now() - t0)} ms, and moves nothing drawn`, keyOf(firstDrawn(c)) === first && toTheEnd(c) < 40, `first ${first} then ${keyOf(firstDrawn(c))}, ${Math.round(toTheEnd(c))} px from the end`);
});

await step("scrolling up draws earlier items, the place kept", async () => {
  const pane = activePane();
  const c = conversationOf(pane);
  const before = all("[data-item]", pane).length;
  check("above what is drawn, a button draws earlier messages", !!withText("button.link", "Show earlier messages", pane));
  // As a reader scrolls: the wheel first, so the view stops following the end.
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -600, bubbles: true }));
  c.scrollTop = 0;
  const first = firstDrawn(c);
  const y0 = first.getBoundingClientRect().top;
  await until("earlier items", () => all("[data-item]", pane).length > before, 5000);
  await sleep(500);
  // The items drawn above settle to their heights in steps, each step keeping what is
  // in view where it is; tens of pixels is that settling, a jump is the whole height.
  const moved = Math.round(first.getBoundingClientRect().top - y0);
  check(`earlier items are drawn above (${before} to ${all("[data-item]", pane).length})`, first.isConnected && c.scrollTop > 0, `scrollTop ${c.scrollTop}`);
  check(`what was first stays where it was (moved ${moved} px)`, Math.abs(moved) < 100, `moved ${moved} px`);
});

await step("a search hit far back in the long session opens there, and the end is a click away", async () => {
  searchFor("function");
  // The app indexes the long session for search as it starts, from its file.
  const hits = await until("hits in the long session", longHits, 60000).catch(() => null);
  if (!hits) {
    check("the long session has search hits", false, "none within 60 s");
    searchFor("");
    return;
  }
  const far = hits.reduce((a, b) => (Number(a.dataset.seq) < Number(b.dataset.seq) ? a : b));
  const seq = Number(far.dataset.seq);
  const t0 = performance.now();
  far.click();
  const pane = activePane();
  const found = await until("the hit drawn and marked", () => pane.querySelector(".found"), 15000).catch(() => null);
  const ms = Math.round(performance.now() - t0);
  const c = conversationOf(pane);
  const r = found?.getBoundingClientRect();
  const v = c.getBoundingClientRect();
  check(`the hit at seq ${seq} is drawn and in view after ${ms} ms`, !!r && r.bottom > v.top && r.top < v.bottom, r ? `${Math.round(r.top)} to ${Math.round(r.bottom)} in ${Math.round(v.top)} to ${Math.round(v.bottom)}` : "not drawn");
  const drawn = all("[data-item]", pane).length;
  check(`it draws what is around it, not all that follows (${drawn})`, drawn > 0 && drawn < 1500, `${drawn} items`);
  const toEnd = withText("button.link", "Go to the end", pane);
  check("below it, a button goes back to the end", !!toEnd);
  toEnd?.click();
  await until("back at the end", () => !withText("button.link", "Go to the end", pane) && toTheEnd(c) < 40, 10000).catch(() => null);
  check("Go to the end draws the end again", !withText("button.link", "Go to the end", pane) && toTheEnd(c) < 40, `${Math.round(toTheEnd(c))} px from the end`);
  searchFor("");
  await until("the search cleared", () => all(".session-row").length >= 30, 5000);
});

await step("the palette stays short with many skills and sessions", async () => {
  openSession("Work 30");
  await until("a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
  const box = await openPalette("");
  const rest = all(".palette-heading").map((h) => h.textContent);
  check("before typing, only the short sections", rest.every((h) => ["Waiting for you", "Actions", "Recent sessions", "Go to"].includes(h)), rest.join(", "));
  check("at most five recent sessions", all('.palette-section[aria-label="Recent sessions"] .palette-item').length <= 5);
  for (const q of ["se", "data", "review", "a"]) {
    await openPalette(q);
    const big = all(".palette-section").filter((s) => s.querySelectorAll(".palette-item").length > 6);
    check(`a search for "${q}" shows a few of each group`, big.length === 0, big.map((s) => s.getAttribute("aria-label")).join(", "));
  }
  await openPalette("/");
  const slash = all(".palette-heading").map((h) => h.textContent);
  check("a slash lists only the agent's commands", slash.every((h) => h === "Agent commands"), slash.join(", "));
  box.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  await until("the palette to close", () => !document.querySelector(".palette"), 4000);
});

await step("the demo runs the tour, not the default profile's agent", async () => {
  await openPalette("Try the demo");
  // Calls to the engine slower than a second while it waits, for a miss to name.
  const slow = [];
  const realFetch = window.fetch;
  window.fetch = async (u, o) => {
    const at = performance.now();
    try {
      return await realFetch(u, o);
    } finally {
      const ms = performance.now() - at;
      if (ms > 1000) slow.push(`${Math.round(ms)} ms: ${String(o?.body ?? u).slice(0, 80)}`);
    }
  };
  (await until("the palette entry", () => withText(".palette-item", "Try the demo"), 4000)).click();
  // Within a second on a Mac; CI's runner, ten times slower at the long session above,
  // took more than 20 seconds. The time is printed, and a miss says what was there.
  const t0 = performance.now();
  const banner = await until("the demo banner", () => withText(".demo-banner", "This is the demo"), 90000)
    .catch((e) => {
      const ws = document.querySelector(".workspace");
      const row = all(".session-row").find((r) => r.textContent.includes("Demo: your policy at work"));
      throw new Error(
        `${e.message}; title "${document.title}", ` +
          `workspace ${ws ? `"${ws.textContent.replace(/\s+/g, " ").slice(0, 60)}"` : "not drawn"}, ` +
          `empty panes ${all(".pane-empty").length}, sessions shown "${all(".session-titleline h2").map((h) => h.textContent).join("|")}", ` +
          `the demo ${row ? `listed, ${row.getAttribute("aria-current") === "true" ? "open" : "not open"}, ${row.hasAttribute("data-shown") ? "in a pane" : "in no pane"}` : "not listed"}, ` +
          `errors "${all(".banner.error").map((b) => b.textContent.trim()).join(" | ")}", slow calls [${slow.join("; ")}]`,
      );
    })
    .finally(() => {
      window.fetch = realFetch;
    });
  check(`the demo has its banner, after ${Math.round(performance.now() - t0)} ms`, !!banner);
  const pane = activePane();
  const header = pane.querySelector(".session-header")?.textContent ?? "";
  check("the demo runs on the tour", header.includes("freeloop-tour") && !/OpenCode\s·/.test(header), header.slice(0, 120));
  const asked = [];
  await until("the closing message", () => {
    for (const b of all(".approval-buttons button", pane)) {
      if (b.textContent.includes("Allow once") && !b.disabled) {
        asked.push(b.closest(".approval").textContent);
        b.click();
      }
    }
    return withText(".msg-agent, .md", "That is the tour", pane);
  }, 45000);
  check("the tour asks about notes.md and .env", asked.length === 2 && asked.some((t) => t.includes("notes.md")) && asked.some((t) => t.includes(".env")), asked.map((t) => t.slice(0, 60)).join(" | "));
  const refused = all(".conversation .decision-denied", pane).map((d) => d.textContent.trim());
  check("git push is refused by the rule, over the global allow", refused.includes("denied by rule"), refused.join(", "));
  check("the hook is refused as a protected path", refused.includes("denied by protected_path"), refused.join(", "));

  const real = window.fetch;
  window.fetch = (u, o) =>
    decodeURIComponent(`${u}${o?.body ?? ""}`).includes("Remove the demo?")
      ? Promise.resolve(new Response(JSON.stringify("OK"), { status: 200, headers: { "Content-Type": "application/json" } }))
      : real(u, o);
  try {
    withText(".demo-banner button", "Remove demo").click();
    await until("the demo to go", () => !all(".session-row").some((r) => r.textContent.includes("Demo: your policy at work")), 8000);
    check("the demo removes itself", true);
  } finally {
    window.fetch = real;
  }
});

await step("a new session starts from an external default profile", async () => {
  await openPalette("New session");
  (await until("the entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "New session"), 4000)).click();
  const start = await until("the new-session form", () => withText("button", "Start session"), 8000);
  const form = start.closest("form");
  await sleep(500);
  const errors = all(".banner.error").map((b) => b.textContent.trim());
  check("the form opens without an error", errors.length === 0, errors.join(" | "));
  const profile = all("label", form).find((l) => l.firstChild?.textContent?.trim() === "Profile")?.querySelector("select")?.selectedOptions?.[0]?.textContent ?? "";
  check("the form opens on the default profile", /OpenCode/i.test(profile), profile);
  // With a project chosen, the default agent can start, or the form says why not.
  const project = all("label", form).find((l) => l.firstChild?.textContent?.trim() === "Project")?.querySelector("select");
  Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(project, project.options[1].value);
  project.dispatchEvent(new Event("change", { bubbles: true }));
  await sleep(500);
  const cannot = form.querySelector(".error-text")?.textContent ?? "";
  check("the default agent is ready, or the form says why not", !start.disabled || /cannot start/.test(cannot), cannot || "disabled without a reason");
  openSession("Work 30");
  await until("back in a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
});

// With no project holding a backlog, the Board says so in two lines and offers to start a
// workflow, or to read how they work (GUI-016).
await step("an empty Board offers to start a workflow", async () => {
  named(".footer-links button", "Board").click();
  const page = await until("the Board", () => document.querySelector(".board-page"), 8000);
  check("an empty Board says there is no backlog and how one comes", page.textContent.includes("No project has a backlog yet.") && page.textContent.includes("A workflow turns a brief into those issues"), page.textContent.slice(0, 120));
  check("and offers Start a workflow and the help", !!withText("button", "Start a workflow…", page) && !!withText("button", "How workflows work", page));
  withText("button", "How workflows work", page).click();
  await until("the workflows help", () => document.querySelector(".help-page h1")?.textContent === "Workflows", 8000);
  check("the link opens the Workflows help", document.querySelector(".help-page h1")?.textContent === "Workflows");
  named(".footer-links button", "Board").click();
  withText("button", "Start a workflow…", await until("the Board again", () => document.querySelector(".board-page"), 8000)).click();
  const form = await until("the new-session form", () => withText("label", "Workflow")?.querySelector("select"), 8000);
  check("Start a workflow opens a new session with the workflow chosen", form.value === "plan-slice-afk", form.value);
  openSession("Work 30");
  await until("back in a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
});

await step("every control has a name", async () => {
  const unnamed = [];
  for (const e of all("button, a[href], summary, [role=tab], [role=menuitem]")) {
    if (!e.getClientRects().length || e.closest("[aria-hidden=true]")) continue;
    if (!(e.getAttribute("aria-label") || e.getAttribute("aria-labelledby") || e.textContent.trim())) unnamed.push(e.outerHTML.slice(0, 120));
  }
  check("every control has an accessible name", unnamed.length === 0, unnamed.join(" | "));
});

return results;
