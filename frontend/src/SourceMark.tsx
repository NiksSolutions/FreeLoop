import type { CSSProperties } from "react";
import { t } from "./i18n.ts";
import { sourceColor, type Source } from "./source.ts";
import VendorLogo, { hasLogo } from "./VendorLogo.tsx";

// SourceMark is the chip that says what a session runs on (GUI-034): the picture the user
// chose, else the vendor's logo where there is one, else two letters on the source's colour.
export default function SourceMark({ source, detail }: { source: Source; detail?: string }) {
  const name = t("marks.provider", { name: source.name }) + (detail ? ` · ${detail}` : "");
  const kind = source.icon ? "source-mark source-mark-picture" : hasLogo(source.vendor) ? "source-mark source-mark-logo" : "source-mark";
  return (
    <span className={kind} role="img" aria-label={name} title={name} style={{ "--source": sourceColor(source.slot) } as CSSProperties}>
      {source.icon ? <img src={source.icon} alt="" draggable={false} /> : hasLogo(source.vendor) ? <VendorLogo vendor={source.vendor} /> : <span aria-hidden="true">{source.letters}</span>}
    </span>
  );
}
