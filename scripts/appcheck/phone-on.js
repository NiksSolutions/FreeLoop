// Turns the phone server on from Settings → Phone, before the phone pass (phone.js):
// a server turned on in a file would wait for an approval this check cannot give. The
// phones' passkeys are made for phone.test (appcheck.sh), so that is the name.
await until("the session list", () => withText(".session-row", "Check phone session"));
await step("the phone server is on for the phone pass", async () => {
  await openPalette("phone");
  (await until("the palette entry", () => all(".palette-item").find((e) => e.textContent.trim().startsWith("Phone")), 3000)).click();
  const page = await until("the phone page", () => all(".page").find((p) => p.querySelector("h2")?.textContent === "Phone"), 5000);
  const status = () => page.querySelector('[role="status"]')?.textContent ?? "";
  if (status() === "Off.") withText("label", "Answer my phone", page).querySelector("input").click();
  await until("the server on", () => /^On, on this computer's port \d+\./.test(status()), 8000);
  check("the phone server is on for the phone pass", /^On, on this computer's port \d+\./.test(status()), status());
  const host = withText("label", "The name your phone reaches", page).querySelector("input");
  if (host.value !== "phone.test") {
    host.focus();
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(host, "phone.test");
    host.dispatchEvent(new Event("input", { bubbles: true }));
    host.blur();
    await sleep(500);
  }
  // OQ-052: the list says which phones have a passkey, and which cannot answer yet.
  const row = (name) => [...page.querySelectorAll(".phone-devices li")].find((li) => li.querySelector("input")?.value === name);
  await until("the phones", () => row("Check old iPhone"), 5000).catch(() => null);
  check("a phone with a passkey says so", !!row("Check iPhone")?.textContent.includes("has a passkey"), row("Check iPhone")?.textContent);
  check("one paired before passkeys says it cannot answer from the phone yet", !!row("Check old iPhone")?.textContent.includes("no passkey yet: cannot answer from the phone"), row("Check old iPhone")?.textContent);
});
return results;
