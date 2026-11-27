import { EngineService, type Config } from "./api.ts";
import { createSaver } from "./configSave.ts";

// The window's one writer of config.yaml (issue 091, configSave.ts). The app takes
// up what the engine answers, shows a change as soon as it waits, and reads the
// settings again when one fails, through onSettings.
let app: { saved: (c: Config) => void; queued: () => void; failed: () => void } = { saved: () => {}, queued: () => {}, failed: () => {} };

export const settingsSaver = createSaver<Config>(
  (patch) => EngineService.ChangeSettings(JSON.stringify(patch)),
  (c) => app.saved(c),
  { queued: () => app.queued(), failed: () => app.failed() },
);

export function onSettings(handlers: typeof app) {
  app = handlers;
}
