// Turns the phone server on from Settings → Phone, before the phone pass (phone.js):
// a server turned on in a file would wait for an approval this check cannot give.
await until("the session list", () => withText(".session-row", "Check phone session"));
await step("the phone server is on for the phone pass", async () => {
  await openPalette("phone");
  (await until("the palette entry", () => all(".palette-item").find((e) => e.textContent.trim().startsWith("Phone")), 3000)).click();
  const page = await until("the phone page", () => all(".page").find((p) => p.querySelector("h2")?.textContent === "Phone"), 5000);
  const status = () => page.querySelector('[role="status"]')?.textContent ?? "";
  if (status() === "Off.") withText("label", "Answer my phone", page).querySelector("input").click();
  await until("the server on", () => /^On, on this computer's port \d+\./.test(status()), 8000);
  check("the phone server is on for the phone pass", /^On, on this computer's port \d+\./.test(status()), status());
});
return results;
