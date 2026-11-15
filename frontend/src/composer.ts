import { t } from "./i18n.ts";

// The message box's placeholder. An empty box's height does not count its placeholder,
// so a pane the user is not in, where the box is narrow and one line, gets the short
// form, whole.
export function placeholder(running: boolean, inPane: boolean): string {
  if (running) return t(inPane ? "composer.placeholder.queue" : "composer.placeholder.queueShort");
  return t(inPane ? "composer.placeholder" : "composer.placeholder.short");
}

export type TabAction = "history" | "command" | "mention" | "suggestion" | "wait" | "move";

// What the box offers when Tab is pressed: the count of each open list (null for the
// earlier prompts' search when it is closed, "loading" for an @ list whose files are on
// their way) and the grey rest of an earlier prompt.
export interface Offer {
  shift: boolean;
  history: number | null;
  commands: number;
  mention: number | "loading" | null;
  rest: string;
}

// tabAction is what Tab does in the message box (GUI-049): it takes what is offered,
// the entry picked in an open list or the grey rest of an earlier prompt, as Enter or
// → do, and waits for an @ list still loading. Only with nothing offered does it move
// the focus on, so the box never holds the keyboard; Shift+Tab always goes back.
export function tabAction(o: Offer): TabAction {
  if (o.shift) return "move";
  if (o.history !== null) return o.history > 0 ? "history" : "move";
  if (o.commands > 0) return "command";
  if (o.mention === "loading") return "wait";
  if (o.mention !== null) return o.mention > 0 ? "mention" : "move";
  return o.rest ? "suggestion" : "move";
}
