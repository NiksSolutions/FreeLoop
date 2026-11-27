package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileFingerprint is one file's fingerprint as Fingerprint has it, or "" when it is not
// there.
func FileFingerprint(dir, rel string) string {
	if fi, err := os.Stat(filepath.Join(dir, rel)); err == nil && fi.Mode().IsRegular() {
		return fmt.Sprintf("%d/%d", fi.Size(), fi.ModTime().UnixNano())
	}
	return ""
}

// Fingerprint describes the hand-editable files under dir, by size and
// modification time, so a poll can tell which of them changed (SET-001).
// Keys are paths relative to dir; a missing file has no key.
func Fingerprint(dir string) map[string]string {
	out := map[string]string{}
	add := func(rel string) {
		if f := FileFingerprint(dir, rel); f != "" {
			out[rel] = f
		}
	}
	add(FileName)
	add(ProvidersFileName)
	add(RulesFileName)
	add(MCPFileName)
	add(ColorsFileName)
	for _, sub := range []struct{ dir, ext string }{
		{EnvironmentsDir, ".yaml"}, {ProfilesDir, ".yaml"}, {PresetsDir, ".yaml"}, {OptimizersDir, ".yaml"}, {WorkflowsDir, ".yaml"},
		{StylesDir, ".json"},
	} {
		if entries, err := os.ReadDir(filepath.Join(dir, sub.dir)); err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), sub.ext) {
					add(filepath.Join(sub.dir, e.Name()))
				}
			}
		}
	}
	return out
}

// Changed returns the keys whose entries differ between two fingerprints.
func Changed(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if before[k] != v {
			out = append(out, k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
