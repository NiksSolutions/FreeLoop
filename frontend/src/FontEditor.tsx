import { useMemo, useState } from "react";
import { t, type PlainKey } from "./i18n.ts";
import { CODE_FONTS, fontOnThisMac, isFontName, isWeight, TEXT_FONTS, WEIGHTS, type FontChoice, type FontWeights } from "./fonts.ts";

// The fonts of Settings → Appearance (GUI-025). It edits ui.textFont,
// ui.codeFont and their weights in config.yaml: a change shows at once and
// reaches the file a moment later, and an edit of the file comes back here.

interface Props {
  textFont: string;
  codeFont: string;
  textWeight: number;
  codeWeight: number;
  onChange: (change: Partial<FontChoice & FontWeights>) => void;
}

export default function FontEditor({ textFont, codeFont, textWeight, codeWeight, onChange }: Props) {
  return (
    <div className="font-editor">
      <FontRow
        label={t("fonts.text")}
        hint={t("fonts.textHint")}
        choices={TEXT_FONTS}
        value={textFont}
        onSet={(v) => onChange({ textFont: v })}
        weight={textWeight}
        onWeight={(w) => onChange({ textWeight: w })}
      />
      <FontRow
        label={t("fonts.code")}
        hint={t("fonts.codeHint")}
        choices={CODE_FONTS}
        value={codeFont}
        onSet={(v) => onChange({ codeFont: v })}
        weight={codeWeight}
        onWeight={(w) => onChange({ codeWeight: w })}
      />
      <span className="muted">{t("fonts.weightHint")}</span>
    </div>
  );
}

const OTHER = "other";

// What each weight is called, as font menus name them.
const WEIGHT_NAMES: Record<number, PlainKey> = {
  100: "fonts.weight.100",
  200: "fonts.weight.200",
  300: "fonts.weight.300",
  400: "fonts.weight.400",
  500: "fonts.weight.500",
  600: "fonts.weight.600",
  700: "fonts.weight.700",
  800: "fonts.weight.800",
  900: "fonts.weight.900",
};

interface RowProps {
  label: string;
  hint: string;
  choices: string[];
  value: string;
  onSet: (v: string) => void;
  weight: number;
  onWeight: (w: number) => void;
}

function FontRow({ label, hint, choices, value, onSet, weight, onWeight }: RowProps) {
  const present = useMemo(() => choices.filter(fontOnThisMac), [choices]);
  // Other was chosen, with no name typed yet.
  const [naming, setNaming] = useState(false);
  // What is being typed. A name with a space on the end is a name on its way,
  // so the field shows this and not the name in force.
  const [draft, setDraft] = useState<string | null>(null);
  const listed = present.includes(value);
  const typed = naming || draft !== null || (value !== "" && !listed);
  const text = draft ?? value;
  const bad = draft !== null && draft.trim() !== "" && !isFontName(draft.trim());
  const onThisMac = useMemo(() => value === "" || fontOnThisMac(value), [value]);
  const missing = !bad && !onThisMac;
  // The app's own weight is Regular; one set in the file that the menu does not
  // offer is listed too, so the menu shows what is in force.
  const shown = isWeight(weight) && weight !== 400 ? weight : 0;
  const offered: number[] = [...WEIGHTS];
  const weights = offered.includes(shown) ? offered : [...offered, shown].sort((a, b) => (a || 400) - (b || 400));
  return (
    <div className="font-row">
      <div className="font-pick">
        <label className="font-family">
          {label}
          <select
            value={typed ? OTHER : value}
            onChange={(e) => {
              const v = e.target.value;
              setDraft(null);
              setNaming(v === OTHER);
              if (v !== OTHER) onSet(v);
            }}
          >
            <option value="">{t("fonts.own")}</option>
            {present.map((f) => (
              <option key={f} value={f}>
                {f}
              </option>
            ))}
            <option value={OTHER}>{t("fonts.other")}</option>
          </select>
        </label>
        <label className="font-weight">
          {t("fonts.weight")}
          <select aria-label={t("fonts.weightOf", { label })} value={shown} onChange={(e) => onWeight(Number(e.target.value))}>
            {weights.map((w) => (
              <option key={w} value={w}>
                {t(WEIGHT_NAMES[w || 400])}
              </option>
            ))}
          </select>
        </label>
      </div>
      {typed && (
        <input
          type="text"
          aria-label={t("fonts.name", { label })}
          aria-invalid={bad || undefined}
          spellCheck={false}
          autoComplete="off"
          placeholder={t("fonts.placeholder")}
          value={text}
          onChange={(e) => {
            const v = e.target.value.trim();
            setDraft(e.target.value);
            if (v === "") onSet("");
            else if (isFontName(v)) onSet(v);
          }}
          onBlur={() => {
            setDraft(null);
            // Listed after all, or nothing typed: the menu says so again.
            setNaming(false);
          }}
        />
      )}
      <span className="muted">{hint}</span>
      {bad && <span role="status" className="font-note">{t("fonts.bad")}</span>}
      {missing && <span role="status" className="font-note">{t("fonts.missing", { font: value })}</span>}
    </div>
  );
}
