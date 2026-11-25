package check_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/check"
	"github.com/ilyachakun/nightshift/internal/rubric"
)

func rubricFindings(t *testing.T, tc string) []check.Finding {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/rubrics/" + tc + "/answers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	judge, err := rubric.LoadScripted(raw)
	if err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS("../../standards")
	cards := func(kind string) ([]rubric.Card, error) { return rubric.Load(fsys, "industry/docs/rubrics", kind) }
	got, err := check.RubricFindings(context.Background(), "../../testdata/rubrics/"+tc, check.DefaultDocKinds, cards, judge)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRubricFindingsWeakPRD(t *testing.T) {
	t.Parallel()
	got := rubricFindings(t, "weak")
	var rules []string
	for _, f := range got {
		rules = append(rules, f.RuleID)
		if f.LayerCaught != "L4" || f.Category != "docs" || f.Fingerprint == "" || f.Location == nil || f.Location.Path != "docs/prd/nightly-export.md" {
			t.Errorf("finding %+v", f)
		}
	}
	slices.Sort(rules)
	want := []string{"docs.prd.adr-consistency", "docs.prd.alternatives", "docs.prd.diagram", "docs.prd.metric"}
	if !slices.Equal(rules, want) {
		t.Fatalf("rules %v, want %v", rules, want)
	}
	for _, f := range got {
		wantSev := check.Block
		if f.RuleID == "docs.prd.diagram" {
			wantSev = check.Warn
		}
		if f.Severity != wantSev {
			t.Errorf("%s severity %s, want %s", f.RuleID, f.Severity, wantSev)
		}
	}
}

func TestRubricFindingsGoodPRD(t *testing.T) {
	t.Parallel()
	if got := rubricFindings(t, "good"); len(got) != 0 {
		t.Fatalf("good documents got findings: %+v", got)
	}
}

func TestRubricFingerprintIgnoresRationale(t *testing.T) {
	t.Parallel()
	a := rubricFindings(t, "weak")
	b := rubricFindings(t, "weak")
	if len(a) == 0 || a[0].Fingerprint != b[0].Fingerprint {
		t.Fatal("fingerprint unstable")
	}
}

func TestRubricFindingsErrors(t *testing.T) {
	t.Parallel()
	bad := func(string) ([]rubric.Card, error) { return nil, os.ErrNotExist }
	if _, err := check.RubricFindings(context.Background(), "../../testdata/rubrics/weak", check.DefaultDocKinds, bad, rubric.Scripted{}); err == nil {
		t.Fatal("cards error swallowed")
	}
	kinds := []check.DocKind{{Name: "prd", Glob: "docs/prd/[.md"}}
	if _, err := check.RubricFindings(context.Background(), ".", kinds, bad, rubric.Scripted{}); err == nil {
		t.Fatal("bad glob accepted")
	}
}
