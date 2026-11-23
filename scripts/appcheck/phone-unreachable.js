// The phone app with the computer out of reach (issue 060), as with Tailscale stopped:
// the page's init script fails every call to the computer until window.__reach is set.
// Helpers are in lib.js.
const banner = () => document.querySelector(".phone-unreachable");

await step("out of reach, the app says so", async () => {
  await until("the message", banner, 10000);
  check("it says the computer cannot be reached, and what to check", banner().textContent.includes("Can't reach your computer") && banner().textContent.includes("Tailscale"), banner()?.textContent);
  check("it offers to try again", !!withText(".phone-unreachable button", "Try again"));
  await window.webkit.messageHandlers.snap?.postMessage("phone-unreachable")?.catch(() => {});
  const main = document.querySelector(".phone-main")?.textContent ?? "";
  check("the lists claim nothing while it is out of reach", !main.includes("Nothing needs you") && !main.includes("No sessions"), main.slice(0, 80));
});

await step("reached again, the message goes by itself", async () => {
  window.__reach = true;
  await until("the message gone", () => !banner(), 20000).catch(() => null);
  check("the message goes once the computer answers", !banner());
  withText(".phone-tabs button", "Sessions")?.click();
  check("and the sessions show", !!(await until("a session", () => document.querySelector(".phone-session"), 5000).catch(() => null)));
});
return results;
