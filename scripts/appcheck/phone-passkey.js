// A phone paired before passkeys (OQ-052), as appcheck.sh lists it: its first screen asks
// it to add one, which the stand-in authenticator makes (phone-authenticator.js) and the
// server checks, and then it has one. Helpers are in lib.js.
const snap = (name) => window.webkit.messageHandlers.snap?.postMessage(name)?.catch(() => {});
const prompt = () => document.querySelector(".phone-main > .phone-passkey");

await step("a phone with no passkey is asked to add one", async () => {
  await until("the passkey prompt", prompt, 10000).catch(() => null);
  check("the first screen asks for a passkey, and says why", !!prompt() && prompt().textContent.includes("need a passkey") && prompt().textContent.includes("18 hours"), prompt()?.textContent);
  const add = prompt() && withText("button", "Add a passkey", prompt());
  check("with a button to add it", !!add);
  await snap("phone-passkey-add");
  await until("the challenge", () => add && !add.disabled, 5000).catch(() => null);
  add?.click();
  await until("the prompt gone", () => !prompt(), 10000).catch(() => null);
  check("once added, the first screen asks no more", !prompt(), prompt()?.textContent);
  const made = window.__passkeys.created[0];
  check(
    "the passkey is made on this phone for the tailnet name, ES256, its user verified, with no attestation",
    window.__passkeys.created.length === 1 &&
      made.rp.id === "phone.test" &&
      made.authenticatorSelection.authenticatorAttachment === "platform" &&
      made.authenticatorSelection.userVerification === "required" &&
      made.attestation === "none" &&
      made.pubKeyCredParams.length === 1 &&
      made.pubKeyCredParams[0].alg === -7,
    JSON.stringify({ made: window.__passkeys.created.length, rp: made?.rp, selection: made?.authenticatorSelection, attestation: made?.attestation }),
  );
});

await step("Settings says the phone has its passkey", async () => {
  withText(".phone-tabs button", "Settings").click();
  const said = await until("the passkey's line", () => withText(".phone-settings .phone-note", "This phone has a passkey"), 5000).catch(() => null);
  check("Settings says the phone has a passkey and when it is asked for again", !!said, document.querySelector(".phone-settings")?.textContent.slice(0, 200));
});
return results;
