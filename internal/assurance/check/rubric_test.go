package check_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/assurance/check"
	"github.com/ilyachakun/nightshift/internal/assurance/rubric"
)

func rubricFindings(t *testing.T, tc string) []check.Finding {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/rubrics/" + tc + "/answers.yaml") //nolint:gosec // G304: tc is a constant of this test
	if err != nil {
		t.Fatal(err)
	}
	judge, err := rubric.LoadScripted(raw)
	if err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS("../../../standards")
	cards := func(kind string) ([]rubric.Card, error) { return rubric.Load(fsys, "industry/docs/rubrics", kind) }
	got, err := check.RubricFindings(t.Context(), "../../../testdata/rubrics/"+tc, check.DefaultDocKinds, cards, judge)
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
	if _, err := check.RubricFindings(t.Context(), "../../../testdata/rubrics/weak", check.DefaultDocKinds, bad, rubric.Scripted{}); err == nil {
		t.Fatal("cards error swallowed")
	}
	kinds := []check.DocKind{{Name: "prd", Glob: "docs/prd/[.md"}}
	if _, err := check.RubricFindings(t.Context(), ".", kinds, bad, rubric.Scripted{}); err == nil {
		t.Fatal("bad glob accepted")
	}
}

type brokenJudge struct{}

func (brokenJudge) Judge(context.Context, rubric.Request) (rubric.Answer, error) {
	return rubric.Answer{}, errors.New("rate limit\nsecond line")
}

func rubricOptions(j rubric.Judge) check.Options {
	fsys := os.DirFS("../../../standards")
	return check.Options{
		Dir: "../../../testdata/rubrics/weak", Docs: check.DefaultDocKinds, Judge: j,
		Cards: func(kind string) ([]rubric.Card, error) { return rubric.Load(fsys, "industry/docs/rubrics", kind) },
	}
}

func TestRunAddsTheRubricFindingsWhenAJudgeIsSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../testdata/rubrics/weak/answers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	judge, err := rubric.LoadScripted(raw)
	if err != nil {
		t.Fatal(err)
	}
	o := rubricOptions(judge)
	o.Out = t.TempDir()
	rep, err := check.Run(t.Context(), o)
	if err != nil || !slices.ContainsFunc(rep.New, func(f check.Finding) bool { return f.RuleID == "docs.prd.metric" }) {
		t.Fatalf("findings %+v, err %v", rep.New, err)
	}
	o.Judge = nil
	rep, err = check.Run(t.Context(), o)
	if err != nil || slices.ContainsFunc(rep.New, func(f check.Finding) bool { return f.RuleID == "docs.prd.metric" }) {
		t.Fatalf("without a judge no rubric may run: %+v, err %v", rep.New, err)
	}
}

func TestABrokenJudgeWarnsAndNeverBlocks(t *testing.T) {
	t.Parallel()
	o := rubricOptions(brokenJudge{})
	o.Out = t.TempDir()
	rep, err := check.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, f := range rep.New {
		if f.RuleID == "rubric.judge-unavailable" {
			seen = f.Severity == check.Warn && !strings.Contains(f.Message, "second line")
		}
	}
	if !seen {
		t.Fatalf("want one warning that the rubrics did not run: %+v", rep.New)
	}
}

// halfJudge fails the first question of the weak PRD and answers fail to the rest.
type halfJudge struct{ n int }

func (h *halfJudge) Judge(context.Context, rubric.Request) (rubric.Answer, error) {
	h.n++
	if h.n > 3 {
		return rubric.Answer{}, errors.New("rate limit")
	}
	return rubric.Answer{Answer: rubric.Fail, Rationale: "r"}, nil
}

func TestAJudgeFailureKeepsTheFindingsAlreadyMade(t *testing.T) {
	t.Parallel()
	o := rubricOptions(&halfJudge{})
	o.Out = t.TempDir()
	rep, err := check.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	var blocks, warns int
	for _, f := range rep.New {
		if f.LayerCaught == "L4" && f.Severity == check.Block {
			blocks++
		}
		if f.RuleID == "rubric.judge-unavailable" {
			warns++
		}
	}
	if blocks == 0 || warns != 1 {
		t.Fatalf("want the earlier blockers and one warning, got %d and %d: %+v", blocks, warns, rep.New)
	}
}
