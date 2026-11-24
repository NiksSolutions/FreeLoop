import { t } from "./i18n.ts";

// What the engine says a rewind would do to the other sessions in the folder
// (Engine.RewindPreview).
export type RewindPreview = { others: { sessionId: string; title: string }[] | null; busy?: string };

// rewindCheck turns a rewind's preview into what the window does first: refuse, because
// another session is in a turn in the folder, or ask, naming the sessions whose changes
// would go too. A preview that could not be read holds nothing up: the engine checks
// again.
export function rewindCheck(p: RewindPreview | null): { refuse?: string; ask?: string } {
  if (!p) return {};
  if (p.busy) return { refuse: t("session.rewindBusy", { title: p.busy }) };
  const others = p.others ?? [];
  if (others.length === 0) return {};
  return { ask: t("session.rewindOthers", { titles: others.map((o) => `"${o.title}"`).join(", ") }) };
}
