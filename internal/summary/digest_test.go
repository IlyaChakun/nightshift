package summary

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

func TestDigestListsGatedAndUnreviewedMergesFirstWithRevertLinks(t *testing.T) {
	t.Parallel()
	d := Digest{
		Summary: Summary{Since: time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)},
		Merges: []integration.MergedPR{
			{Number: 1, Title: "docs: notes", URL: "https://x/1", Reviewed: true, Reason: "Explains the package."},
			{Number: 2, Title: "feat: schema field", URL: "https://x/2", Reviewed: true, Labels: []string{"needs-owner-review"}, MergedBy: "bot"},
			{Number: 3, Title: "fix: thing", URL: "https://x/3"},
		},
		Decisions: []Decision{{TaskID: 7, Title: "t", Actor: "owner", Event: "approve", From: "spec", To: "queued", At: time.Date(2026, 11, 25, 2, 0, 0, 0, time.UTC)}},
	}
	out := d.Format()
	i2, i3, i1 := strings.Index(out, "#2 "), strings.Index(out, "#3 "), strings.Index(out, "#1 ")
	if !(i2 >= 0 && i2 < i3 && i3 < i1) {
		t.Fatalf("order must be gated, unreviewed, rest:\n%s", out)
	}
	for _, want := range []string{"[gated]", "[no independent review recorded]", "revert: https://x/1", "why: Explains the package.", "(by bot)", "Decisions: 1.", "spec -> queued, by owner"} {
		if !strings.Contains(out, want) {
			t.Errorf("digest lacks %q:\n%s", want, out)
		}
	}
}

func TestDigestSaysWhenTheMergeListCannotBeRead(t *testing.T) {
	t.Parallel()
	out := Digest{MergeErr: errors.New("403")}.Format()
	if !strings.Contains(out, "could not be read") || strings.Contains(out, "nothing") {
		t.Fatalf("an unreadable list must not read as an empty one:\n%s", out)
	}
	if out := (Digest{}).Format(); !strings.Contains(out, "Merged: nothing.") {
		t.Fatalf("an empty window says so:\n%s", out)
	}
}
