import { useRef, useState, type CSSProperties } from "react";
import { t } from "./i18n.ts";
import { PICTURE_TYPES, toMarkIcon } from "./markIcon.ts";
import { assignSlots, defaultLetters, SLOTS, sourceColor, validIcon, validLetters, type Named, type ProviderNamed, type SourceChoices } from "./source.ts";
import SourceMark from "./SourceMark.tsx";
import { agentVendor, providerVendor } from "./vendors.ts";

// SourceMarks chooses each provider's and agent's colour, letters and picture (GUI-034),
// every one the same way. A picture shows before letters typed, and letters typed before
// the vendor's logo.

type Choice = { slot: number; letters: string; icon: string };

export default function SourceMarks({ value, providers, harnesses, onChange }: { value: Record<string, Choice> | null | undefined; providers: readonly ProviderNamed[]; harnesses: readonly Named[]; onChange: (next: Record<string, Choice>) => void }) {
  const list = [...providers.map((p) => ({ ...p, vendor: providerVendor(p) })), ...harnesses.filter((h) => h.id !== "builtin").map((h) => ({ ...h, vendor: agentVendor(h.id) }))];
  const slots = assignSlots(list.map((x) => x.id), value as SourceChoices);
  const set = (id: string, change: Partial<Choice>) => {
    const was = value?.[id] ?? { slot: 0, letters: "", icon: "" };
    const next = { ...was, ...change, icon: (change.icon ?? was.icon) || "" };
    const all = { ...(value ?? {}) };
    if (!next.slot && !next.letters && !next.icon) delete all[id];
    else all[id] = next;
    onChange(all);
  };
  // One file chooser for the list; a ref remembers whose picture it is choosing, as the
  // file can come before the next render.
  const chooser = useRef<HTMLInputElement>(null);
  const choosingFor = useRef("");
  const [unreadable, setUnreadable] = useState("");
  const choose = (id: string) => {
    choosingFor.current = id;
    setUnreadable("");
    chooser.current?.click();
  };
  return (
    <>
      <input
        ref={chooser}
        type="file"
        accept={PICTURE_TYPES}
        hidden
        onChange={(e) => {
          const file = e.target.files?.[0];
          const id = choosingFor.current;
          e.target.value = "";
          if (!file || !id) return;
          toMarkIcon(file)
            .then((icon) => set(id, { icon }))
            .catch(() => setUnreadable(id));
        }}
      />
      <ul className="source-marks">
        {list.map((x) => {
          const letters = value?.[x.id]?.letters ?? "";
          const bad = letters !== "" && !validLetters(letters);
          const icon = validIcon(value?.[x.id]?.icon) ? value![x.id].icon! : null;
          return (
            <li key={x.id}>
              <SourceMark source={{ id: x.id, name: x.name, letters: letters && !bad ? letters : defaultLetters(x.name), slot: slots.get(x.id) ?? 0, vendor: icon || (letters && !bad) ? null : x.vendor, icon }} />
              <span className="source-marks-name">{x.name}</span>
              <span className="source-marks-slots" role="group" aria-label={t("marks.colourFor", { name: x.name })}>
                {Array.from({ length: SLOTS }, (_, i) => i + 1).map((slot) => (
                  <button
                    key={slot}
                    type="button"
                    className="source-marks-slot"
                    aria-label={t("marks.colour", { slot })}
                    aria-pressed={slots.get(x.id) === slot}
                    style={{ "--source": sourceColor(slot) } as CSSProperties}
                    onClick={() => set(x.id, { slot })}
                  />
                ))}
              </span>
              <input
                aria-label={t("marks.lettersFor", { name: x.name })}
                aria-invalid={bad || undefined}
                maxLength={2}
                placeholder={defaultLetters(x.name)}
                value={letters}
                onChange={(e) => set(x.id, { letters: e.target.value })}
              />
              {icon ? (
                <button type="button" className="link" aria-label={t("marks.removePictureFor", { name: x.name })} onClick={() => set(x.id, { icon: "" })}>
                  {t("marks.removePicture")}
                </button>
              ) : (
                <button type="button" className="link" aria-label={t("marks.pictureFor", { name: x.name })} onClick={() => choose(x.id)}>
                  {t("marks.picture")}
                </button>
              )}
              {value?.[x.id] && (
                <button type="button" className="link" aria-label={t("marks.reset", { name: x.name })} onClick={() => set(x.id, { slot: 0, letters: "", icon: "" })}>
                  {t("common.reset")}
                </button>
              )}
              {unreadable === x.id && (
                <span role="status" className="font-note">
                  {t("marks.pictureBad")}
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </>
  );
}
