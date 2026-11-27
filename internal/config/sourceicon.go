package config

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
)

// SourceIconSize is the most pixels a side a mark's picture has: four times the mark,
// for sharp drawing on any screen.
const SourceIconSize = 64

// maxSourceIcon bounds the PNG's bytes, so a picture cannot swell config.yaml.
const maxSourceIcon = 24 << 10

const sourceIconPrefix = "data:image/png;base64,"

// ValidSourceIcon reports whether v is a mark's picture (GUI-034): a data: URL of a PNG
// that decodes, of at most SourceIconSize pixels a side. Only a PNG passes, never an SVG,
// so what reaches the window is pixels and not markup.
func ValidSourceIcon(v string) bool {
	if !strings.HasPrefix(v, sourceIconPrefix) || len(v) > len(sourceIconPrefix)+base64.StdEncoding.EncodedLen(maxSourceIcon) {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(v[len(sourceIconPrefix):])
	if err != nil || len(b) > maxSourceIcon {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	return err == nil && cfg.Width >= 1 && cfg.Height >= 1 && cfg.Width <= SourceIconSize && cfg.Height <= SourceIconSize
}
