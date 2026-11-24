import { useEffect, type RefObject } from "react";
import { tabWrap } from "./a11y.ts";

// controls are what Tab reaches in a dialog: shown and not disabled.
function controls(box: HTMLElement): HTMLElement[] {
  return [...box.querySelectorAll<HTMLElement>('a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"])')].filter((el) => !el.matches(":disabled") && el.tabIndex >= 0 && el.getClientRects().length > 0);
}

// useModalFocus holds the focus in a modal dialog (NFR-A11Y-002): its first control
// when it opens, or the dialog itself while it has none ready, Tab and Shift-Tab kept
// inside it, and the focus back where it was when it closes, unless something it
// opened took it. The dialog's element needs tabIndex -1.
export function useModalFocus(box: RefObject<HTMLElement | null>, ready = true) {
  useEffect(() => {
    const dialog = box.current;
    if (!dialog) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Tab" || e.defaultPrevented) return;
      const list = controls(dialog);
      const to = tabWrap(list.indexOf(document.activeElement as HTMLElement), list.length, e.shiftKey);
      if (to === -1) return;
      e.preventDefault();
      if (to !== null) list[to].focus();
    };
    dialog.addEventListener("keydown", onKey);
    return () => {
      dialog.removeEventListener("keydown", onKey);
      const now = document.activeElement;
      if (opener?.isConnected && (!now || now === document.body || dialog.contains(now))) opener.focus();
    };
  }, [box]);
  useEffect(() => {
    const dialog = box.current;
    if (!dialog) return;
    const now = document.activeElement;
    if (now && now !== dialog && dialog.contains(now)) return;
    const first = ready ? controls(dialog)[0] : undefined;
    if (first) first.focus();
    else dialog.focus();
  }, [box, ready]);
}
