package dispatch

import "testing"

func TestIsPatchPath(t *testing.T) {
	t.Parallel()
	for p, want := range map[string]bool{
		"patch.diff":              true, // recorded by workers before per-attempt directories
		"a1b2c3d4/patch.diff":     true,
		"check/report.sarif":      false,
		"a/b/patch.diff":          false,
		"../patch.diff":           false,
		"./patch.diff":            false,
		"/patch.diff":             false,
		"a1b2c3d4/patch.diff.bak": false,
	} {
		if got := isPatchPath(p); got != want {
			t.Errorf("isPatchPath(%q) = %v, want %v", p, got, want)
		}
	}
}
