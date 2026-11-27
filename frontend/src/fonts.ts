// The user's fonts (GUI-025), from config.yaml or the editor in Settings. Kept
// apart from the components so it can be tested.

export interface FontChoice {
  textFont: string;
  codeFont: string;
}

// The fonts' weights, as CSS counts them; 0 is the app's own, 400. Like the text
// size they are the user's, not a style's (GUI-053).
export interface FontWeights {
  textWeight: number;
  codeWeight: number;
}

// What the editor offers: the app's own (Regular) and the steps on either side
// that fonts commonly have. config.yaml takes any of 100 to 900 in steps of 100.
export const WEIGHTS = [300, 0, 500, 600] as const;

// The same check as config.ValidWeight.
export function isWeight(w: unknown): w is number {
  return typeof w === "number" && Number.isInteger(w) && w >= 100 && w <= 900 && w % 100 === 0;
}

// What the editor offers, as far as they are on this computer: what macOS, Windows
// and the common Linux desktops come with, then fonts people commonly install. Any
// other name can be typed.
export const TEXT_FONTS = [
  "Helvetica Neue",
  "Avenir Next",
  "Arial",
  "Verdana",
  "Gill Sans",
  "Optima",
  "Charter",
  "Georgia",
  "Iowan Old Style",
  "Palatino",
  "Segoe UI",
  "Calibri",
  "Ubuntu",
  "Cantarell",
  "Noto Sans",
  "DejaVu Sans",
  "Inter",
  "IBM Plex Sans",
  "Manrope",
  "Open Sans",
  "Roboto",
  "Source Sans 3",
];

export const CODE_FONTS = [
  "Menlo",
  "Monaco",
  "SF Mono",
  "Courier New",
  "Andale Mono",
  "PT Mono",
  "Consolas",
  "Cascadia Mono",
  "DejaVu Sans Mono",
  "Liberation Mono",
  "Noto Sans Mono",
  "Cascadia Code",
  "Fira Code",
  "Hack",
  "IBM Plex Mono",
  "Inconsolata",
  "Iosevka",
  "JetBrains Mono",
  "Source Code Pro",
  "Ubuntu Mono",
  "Victor Mono",
  // Nerd Fonts, for a prompt's icons in the terminal (GUI-054): the names
  // their installers give the commonest ones.
  "JetBrainsMono Nerd Font",
  "FiraCode Nerd Font",
  "Hack Nerd Font",
  "MesloLGS NF",
  "MesloLGS Nerd Font",
  "CaskaydiaCove Nerd Font",
  "SauceCodePro Nerd Font",
  "Iosevka Nerd Font",
];

// The same check as config.ValidFontName. A name only reaches the page after
// this, so nothing but a family name gets in.
export function isFontName(v: string): boolean {
  return [...v].length <= 64 && /^[\p{L}\p{N}](?:[\p{L}\p{N}\p{M} ._+-]*[\p{L}\p{N}\p{M}._+-])?$/u.test(v);
}

// fontStack is the value for a font's variable: the name, then the app's own
// fonts for a computer without it. Null when there is no name to use.
export function fontStack(name: string | undefined, fallback: string): string | null {
  return name && isFontName(name) ? `"${name}", var(--${fallback})` : null;
}

// applyFonts puts the fonts and their weights on the page. Without a name or a
// weight the variable goes back to what app.css gives it.
export function applyFonts(style: Pick<CSSStyleDeclaration, "setProperty" | "removeProperty">, fonts: Partial<FontChoice & FontWeights>) {
  for (const [property, name, fallback] of [
    ["--font-text", fonts.textFont, "font-text-default"],
    ["--mono", fonts.codeFont, "mono-default"],
  ] as const) {
    const stack = fontStack(name, fallback);
    if (stack) style.setProperty(property, stack);
    else style.removeProperty(property);
  }
  for (const [property, weight] of [
    ["--weight-text", fonts.textWeight],
    ["--weight-code", fonts.codeWeight],
  ] as const) {
    if (isWeight(weight)) style.setProperty(property, String(weight));
    else style.removeProperty(property);
  }
}

const GENERIC = ["monospace", "serif", "sans-serif"];

// fontPresent tells whether a family is on this computer: a page cannot list fonts
// or ask, but text in a missing family measures the same as its fallback, for
// every fallback. measure gives the width of a sample in a CSS font. It says
// yes when nothing can be measured, so a name is never held back on a guess.
export function fontPresent(name: string, measure: (font: string) => number): boolean {
  if (!measure(`72px ${GENERIC[0]}`)) return true;
  return GENERIC.some((g) => measure(`72px "${name}", ${g}`) !== measure(`72px ${g}`));
}

// fontOnThisMac is fontPresent on the page. It measures text in the page, not
// on a canvas: WebKit's canvas does not see fonts the user installed, though
// the page draws them.
export function fontOnThisMac(name: string): boolean {
  if (!isFontName(name)) return false;
  const probe = document.createElement("span");
  probe.textContent = "mmmmmmmmmmlli 0123456789 WwQq{}";
  probe.setAttribute("aria-hidden", "true");
  probe.style.cssText = "position: absolute; visibility: hidden; white-space: nowrap";
  document.body.append(probe);
  const present = fontPresent(name, (font) => {
    probe.style.font = font;
    return probe.getBoundingClientRect().width;
  });
  probe.remove();
  return present;
}
