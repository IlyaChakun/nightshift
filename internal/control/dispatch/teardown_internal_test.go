package dispatch

import (
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/flow"
)

// Every state has one answer to "what happens to its preview": torn down, kept for
// preview_hours, in use, or none yet. A new state fails here until it is placed.
func TestEveryStateHasOnePreviewRule(t *testing.T) {
	inUse := []string{string(flow.Preview), string(flow.Acceptance)}
	beforePreview := []string{string(flow.Idea), string(flow.Grilling), string(flow.DocChecks), string(flow.Spec), string(flow.DesignReview)}
	for _, s := range flow.Names(flow.Every) {
		n := 0
		for _, group := range [][]string{stalePreviewStates, previewKeptStates, inUse, beforePreview} {
			if slices.Contains(group, s) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("state %s is in %d preview groups, want exactly 1", s, n)
		}
	}
}
