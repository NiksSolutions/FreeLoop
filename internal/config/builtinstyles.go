package config

import (
	"maps"
	"slices"
	"strings"
)

// The styles the app ships (GUI-053), listed under Style in Settings → Appearance
// beside the user's own. They are values in the code, not files: they come with
// each version, a backup does not carry them, and they cannot be changed or
// deleted, only saved as a new style and changed there. A file in styles/ with one's
// ID takes its place, as a preset's file does. Each sets every color the editor
// offers in both appearances and leaves the fonts the app's own; the test holds
// them to WCAG AA's contrast. Where a theme's own color reads too faintly on its
// background, a deeper or lighter shade of the same hue stands in.
var builtinStyles = []Style{
	{
		// After the user's Obsidian theme, Niks Theme V2: charcoal greys, a sky-blue
		// accent and peach links in the dark, GitHub's greys and blue in the light.
		ID: "niks", Name: "Niks Theme",
		Dark: map[string]string{
			"bg-app": "#262626", "bg": "#1a1a1a", "surface": "#222222", "surface-2": "#363636", "input": "#141414",
			"border": "#363636", "border-strong": "#4a4a4a", "input-border": "#6a6a6a",
			"text": "#d6d6d6", "muted": "#969696", "faint": "#858585",
			"accent": "#57a5ff", "accent-text": "#141414", "link": "#ffd087", "focus": "#57a5ff",
			"ok": "#7ee787", "warn": "#d29922", "danger": "#f47067", "git": "#d2a8ff",
		},
		Light: map[string]string{
			"bg-app": "#f6f8fa", "bg": "#ffffff", "surface": "#f6f8fa", "surface-2": "#eaeef2", "input": "#ffffff",
			"border": "#d0d7de", "border-strong": "#afb8c1", "input-border": "#7d8590",
			"text": "#24292f", "muted": "#57606a", "faint": "#6e7781",
			"accent": "#0a6cdc", "accent-text": "#ffffff", "link": "#0a6cdc", "focus": "#0a6cdc",
			"ok": "#1a7f37", "warn": "#9a6700", "danger": "#cf222e", "git": "#8250df",
		},
	},
	{
		// Ethan Schoonover's Solarized: base03 to base3 and its accents.
		ID: "solarized", Name: "Solarized",
		Dark: map[string]string{
			"bg-app": "#00212b", "bg": "#002b36", "surface": "#073642", "surface-2": "#0d4250", "input": "#00212b",
			"border": "#0b3f4c", "border-strong": "#28525d", "input-border": "#657b83",
			"text": "#93a1a1", "muted": "#839496", "faint": "#7f9194",
			"accent": "#268bd2", "accent-text": "#001b22", "link": "#4aa3e0", "focus": "#268bd2",
			"ok": "#859900", "warn": "#b58900", "danger": "#e0524f", "git": "#8a8fd6",
		},
		Light: map[string]string{
			"bg-app": "#eee8d5", "bg": "#fdf6e3", "surface": "#f5efdc", "surface-2": "#e6dfc9", "input": "#fdf6e3",
			"border": "#e4ddc8", "border-strong": "#cfc7b0", "input-border": "#657b83",
			"text": "#2f4a52", "muted": "#52676e", "faint": "#586e75",
			"accent": "#1f6fa8", "accent-text": "#ffffff", "link": "#1f6fa8", "focus": "#268bd2",
			"ok": "#5f6e00", "warn": "#8a6800", "danger": "#c42b28", "git": "#5b5fb3",
		},
	},
	{
		// Arctic Ice Studio's Nord: Polar Night in the dark, Snow Storm in the light,
		// Frost for the accent and Aurora for the rest.
		ID: "nord", Name: "Nord",
		Dark: map[string]string{
			"bg-app": "#272c36", "bg": "#2e3440", "surface": "#3b4252", "surface-2": "#434c5e", "input": "#272c36",
			"border": "#3b4252", "border-strong": "#4c566a", "input-border": "#6c7a96",
			"text": "#eceff4", "muted": "#d8dee9", "faint": "#a9b3c4",
			"accent": "#88c0d0", "accent-text": "#2e3440", "link": "#88c0d0", "focus": "#88c0d0",
			"ok": "#a3be8c", "warn": "#ebcb8b", "danger": "#d57780", "git": "#b48ead",
		},
		Light: map[string]string{
			"bg-app": "#e5e9f0", "bg": "#eceff4", "surface": "#e5e9f0", "surface-2": "#d8dee9", "input": "#f6f7fa",
			"border": "#d8dee9", "border-strong": "#c2cad8", "input-border": "#7b88a1",
			"text": "#2e3440", "muted": "#434c5e", "faint": "#4c566a",
			"accent": "#4c6d97", "accent-text": "#ffffff", "link": "#4c6d97", "focus": "#5e81ac",
			"ok": "#4e7a3a", "warn": "#8a6512", "danger": "#a5444d", "git": "#8a5f86",
		},
	},
	{
		// Pavel Pertsev's Gruvbox, with its blue for the accent.
		ID: "gruvbox", Name: "Gruvbox",
		Dark: map[string]string{
			"bg-app": "#1d2021", "bg": "#282828", "surface": "#3c3836", "surface-2": "#504945", "input": "#1d2021",
			"border": "#3c3836", "border-strong": "#504945", "input-border": "#7c6f64",
			"text": "#ebdbb2", "muted": "#d5c4a1", "faint": "#a89984",
			"accent": "#83a598", "accent-text": "#1d2021", "link": "#83a598", "focus": "#83a598",
			"ok": "#b8bb26", "warn": "#fabd2f", "danger": "#fb4934", "git": "#d3869b",
		},
		Light: map[string]string{
			"bg-app": "#f2e5bc", "bg": "#fbf1c7", "surface": "#f2e5bc", "surface-2": "#ebdbb2", "input": "#f9f5d7",
			"border": "#ebdbb2", "border-strong": "#d5c4a1", "input-border": "#928374",
			"text": "#3c3836", "muted": "#504945", "faint": "#665c54",
			"accent": "#076678", "accent-text": "#ffffff", "link": "#076678", "focus": "#076678",
			"ok": "#79740e", "warn": "#b57614", "danger": "#9d0006", "git": "#8f3f71",
		},
	},
	{
		// Rosé Pine in the dark and Rosé Pine Dawn in the light.
		ID: "rose-pine", Name: "Rosé Pine",
		Dark: map[string]string{
			"bg-app": "#191724", "bg": "#1f1d2e", "surface": "#26233a", "surface-2": "#403d52", "input": "#191724",
			"border": "#26233a", "border-strong": "#403d52", "input-border": "#6e6a86",
			"text": "#e0def4", "muted": "#908caa", "faint": "#8a86a3",
			"accent": "#c4a7e7", "accent-text": "#191724", "link": "#c4a7e7", "focus": "#c4a7e7",
			"ok": "#9ccfd8", "warn": "#f6c177", "danger": "#eb6f92", "git": "#ebbcba",
		},
		Light: map[string]string{
			"bg-app": "#f2e9e1", "bg": "#faf4ed", "surface": "#fffaf3", "surface-2": "#f4ede8", "input": "#fffaf3",
			"border": "#dfdad9", "border-strong": "#cecacd", "input-border": "#797593",
			"text": "#575279", "muted": "#686480", "faint": "#6e6a86",
			"accent": "#286983", "accent-text": "#ffffff", "link": "#286983", "focus": "#907aa9",
			"ok": "#286983", "warn": "#a4650f", "danger": "#b4637a", "git": "#907aa9",
		},
	},
	{
		// Zeno Rocha's Dracula in the dark and its light twin Alucard, after the
		// palettes at draculatheme.com/spec: purple for the accent, cyan links, pink
		// for git. Comment's blue-grey is lightened for muted text in the dark.
		// The usage charts take Dracula's purple, green, cyan, orange and pink, in that
		// order in both appearances, the order in which neighbours in a stack stay apart
		// with any colour vision (adjacent CVD ΔE at least 16.4 dark and 12.4 light,
		// plain 20.1 and 15.7; orange beside green was 3.3). In the dark they are
		// Dracula's own neon, cyan given more colour, brighter than the band chart marks
		// usually keep, at the user's request that they pop; in the light, Alucard's.
		// "Other" is a quieter grey, apart from the pink beside it.
		ID: "dracula", Name: "Dracula",
		Dark: map[string]string{
			"bg-app": "#21222c", "bg": "#282a36", "surface": "#343746", "surface-2": "#424450", "input": "#21222c",
			"border": "#343746", "border-strong": "#44475a", "input-border": "#6272a4",
			"text": "#f8f8f2", "muted": "#a9b1d6", "faint": "#99a2cc",
			"accent": "#bd93f9", "accent-text": "#282a36", "link": "#8be9fd", "focus": "#bd93f9",
			"ok": "#50fa7b", "warn": "#ffb86c", "danger": "#ff5555", "git": "#ff79c6",
			"series-1": "#bc92f8", "series-2": "#4bf677", "series-3": "#64e5ff", "series-4": "#ffbb73", "series-5": "#ff7dc7", "series-other": "#7c7f94",
		},
		Light: map[string]string{
			"bg-app": "#ece9df", "bg": "#fffbeb", "surface": "#f7f3e3", "surface-2": "#dedccf", "input": "#fffbeb",
			"border": "#dedccf", "border-strong": "#ceccc0", "input-border": "#8f8970",
			"text": "#1f1f1f", "muted": "#58533d", "faint": "#6c664b",
			"accent": "#644ac9", "accent-text": "#ffffff", "link": "#036a96", "focus": "#644ac9",
			"ok": "#14710a", "warn": "#a34d14", "danger": "#cb3a2a", "git": "#a3144d",
			"series-1": "#644ac9", "series-2": "#106e05", "series-3": "#056893", "series-4": "#ad5620", "series-5": "#9d0848", "series-other": "#8f8970",
		},
	},
}

// BuiltinStyles returns the styles the app ships, each its own copy.
func BuiltinStyles() []Style {
	out := make([]Style, len(builtinStyles))
	for i, s := range builtinStyles {
		s.Version, s.Builtin = 1, true
		s.Light, s.Dark = maps.Clone(s.Light), maps.Clone(s.Dark)
		out[i] = s
	}
	return out
}

// IsBuiltinStyle reports whether the app ships a style with this ID.
func IsBuiltinStyle(id string) bool {
	return slices.ContainsFunc(builtinStyles, func(s Style) bool { return s.ID == id })
}

// StylesWithBuiltins is the list Settings shows: the built-in styles with the files
// read from styles/ on top, sorted by name.
func StylesWithBuiltins(files []Style) []Style {
	out := slices.DeleteFunc(BuiltinStyles(), func(b Style) bool {
		return slices.ContainsFunc(files, func(f Style) bool { return f.ID == b.ID })
	})
	out = append(out, files...)
	slices.SortStableFunc(out, func(a, b Style) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
