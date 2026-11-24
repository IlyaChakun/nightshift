package policy_test

import (
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/policy"
)

const rulesYAML = `
levels:
  critical:
    paths: ["**/migrations/**", "**/billing/**", ".github/**"]
    tags: [payments, auth]
    signals: { new_dependency: true }
  high:
    paths: ["deploy/**", "docs/adr/**"]
    tags: [architecture]
    signals: { new_module: true, diff_lines_over: 400 }
  low:
    paths: ["**/*_test.go", "docs/guides/**"]
    max_diff_lines: 80
`

func rules(t *testing.T) *policy.Rules {
	t.Helper()
	r, err := policy.ParseRules([]byte(rulesYAML), config.NewValidator())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestClassify(t *testing.T) {
	t.Parallel()
	r := rules(t)
	for name, tc := range map[string]struct {
		c    policy.Change
		want policy.Level
	}{
		"unmatched is medium":                {policy.Change{Paths: []string{"internal/app/app.go"}, DiffLines: 30}, policy.Medium},
		"only tests and small is low":        {policy.Change{Paths: []string{"internal/app/app_test.go", "docs/guides/a.md"}, DiffLines: 40}, policy.Low},
		"a test change that is big is not":   {policy.Change{Paths: []string{"a_test.go"}, DiffLines: 500}, policy.High},
		"a mixed change is not low":          {policy.Change{Paths: []string{"a_test.go", "a.go"}, DiffLines: 10}, policy.Medium},
		"a low diff over the limit":          {policy.Change{Paths: []string{"a_test.go"}, DiffLines: 81}, policy.Medium},
		"a migration is critical":            {policy.Change{Paths: []string{"db/migrations/0001.sql"}}, policy.Critical},
		"a root migrations directory":        {policy.Change{Paths: []string{"migrations/0001.sql"}}, policy.Critical},
		"critical beats high":                {policy.Change{Paths: []string{"deploy/x.yaml", ".github/ci.yml"}}, policy.Critical},
		"a tag raises":                       {policy.Change{Paths: []string{"a.go"}, Tags: []string{"payments"}}, policy.Critical},
		"a new dependency is critical":       {policy.Change{Paths: []string{"go.mod"}, NewDependency: true}, policy.Critical},
		"a new module is high":               {policy.Change{Paths: []string{"a.go"}, NewModule: true}, policy.High},
		"a big diff is high":                 {policy.Change{Paths: []string{"a.go"}, DiffLines: 401}, policy.High},
		"an agent can raise":                 {policy.Change{Paths: []string{"a_test.go"}, DiffLines: 5, Floor: policy.High}, policy.High},
		"an agent cannot lower":              {policy.Change{Paths: []string{"deploy/x.yaml"}, Floor: policy.Low}, policy.High},
		"case does not hide a critical path": {policy.Change{Paths: []string{"Db/Migrations/1.sql"}}, policy.Critical},
		"an empty change is medium":          {policy.Change{}, policy.Medium},
	} {
		if got := r.Classify(tc.c); got.Level != tc.want || len(got.Reasons) == 0 {
			t.Errorf("%s: %+v, want %s", name, got, tc.want)
		}
	}
}

func TestReasonsNameTheWinningLevel(t *testing.T) {
	t.Parallel()
	got := rules(t).Classify(policy.Change{Paths: []string{"deploy/x.yaml", ".github/ci.yml"}})
	if !slices.Equal(got.Reasons, []string{"path .github/ci.yml"}) {
		t.Fatalf("reasons: %v", got.Reasons)
	}
}

func TestParseRulesRejectsBadFiles(t *testing.T) {
	t.Parallel()
	v := config.NewValidator()
	for name, doc := range map[string]string{
		"unknown level": "levels: {severe: {paths: [a]}}",
		"unknown field": "levels: {high: {paths: [a], colour: red}}",
		"absolute glob": "levels: {high: {paths: [/etc/**]}}",
		"no levels":     "levels: {}",
		"not even yaml": "levels: [",
	} {
		if _, err := policy.ParseRules([]byte(doc), v); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestNoAgentInputLowersTheLevel is the property of design section 11: more tags, signals and
// floors never lower the result.
func TestNoAgentInputLowersTheLevel(t *testing.T) {
	t.Parallel()
	r := rules(t)
	paths := []string{"a.go", "a_test.go", "deploy/x", "db/migrations/1.sql", "docs/guides/g.md"}
	rapid.Check(t, func(t *rapid.T) {
		base := policy.Change{
			Paths:     rapid.SliceOfNDistinct(rapid.SampledFrom(paths), 0, 3, func(s string) string { return s }).Draw(t, "paths"),
			DiffLines: rapid.IntRange(0, 600).Draw(t, "lines"),
		}
		more := base
		more.Tags = rapid.SliceOf(rapid.SampledFrom([]string{"payments", "architecture", "x"})).Draw(t, "tags")
		more.Floor = rapid.SampledFrom([]policy.Level{"", policy.Low, policy.Medium, policy.High, policy.Critical}).Draw(t, "floor")
		more.NewModule = rapid.Bool().Draw(t, "module")
		if policy.Max(r.Classify(base).Level, r.Classify(more).Level) != r.Classify(more).Level {
			t.Fatalf("extra input lowered %+v to %+v", base, more)
		}
	})
}

func TestDecide(t *testing.T) {
	t.Parallel()
	solo := policy.ReviewPolicy{Mode: policy.ModeBlastRadius, Sample: 0.1}
	for name, tc := range map[string]struct {
		p    policy.ReviewPolicy
		l    policy.Level
		roll float64
		want policy.Action
	}{
		"all reviews everything":          {policy.ReviewPolicy{Mode: policy.ModeAll}, policy.Low, 0.9, policy.ActionReview},
		"blast radius low is auto":        {solo, policy.Low, 0.5, policy.ActionAuto},
		"blast radius medium is notify":   {solo, policy.Medium, 0.5, policy.ActionNotify},
		"blast radius high is review":     {solo, policy.High, 0.5, policy.ActionReview},
		"a sampled low goes to the owner": {solo, policy.Low, 0.05, policy.ActionReview},
		"a sampled high is unchanged":     {solo, policy.High, 0.05, policy.ActionReview},
		"off sends only critical":         {policy.ReviewPolicy{Mode: policy.ModeOff}, policy.High, 0, policy.ActionAuto},
		"critical in off":                 {policy.ReviewPolicy{Mode: policy.ModeOff}, policy.Critical, 0.9, policy.ActionReviewDesignAPR},
		"critical in blast radius":        {solo, policy.Critical, 0.9, policy.ActionReviewDesignAPR},
		"the project overrides a default": {policy.ReviewPolicy{Mode: policy.ModeBlastRadius, Actions: map[policy.Level]policy.Action{policy.Medium: policy.ActionReview}}, policy.Medium, 0.9, policy.ActionReview},
		"critical cannot be overridden":   {policy.ReviewPolicy{Mode: policy.ModeBlastRadius, Actions: map[policy.Level]policy.Action{policy.Critical: policy.ActionAuto}}, policy.Critical, 0.9, policy.ActionReviewDesignAPR},
	} {
		if got, err := tc.p.Decide(tc.l, tc.roll); err != nil || got != tc.want {
			t.Errorf("%s: %q %v, want %q", name, got, err, tc.want)
		}
	}
	if _, err := (policy.ReviewPolicy{Mode: policy.ModeOff, Team: true}).Decide(policy.Low, 0); !errors.Is(err, policy.ErrTeamOff) {
		t.Errorf("team off: %v", err)
	}
	if _, err := solo.Decide("severe", 0); err == nil {
		t.Error("an unknown level was accepted")
	}
	if _, err := (policy.ReviewPolicy{Mode: "sometimes"}).Decide(policy.Low, 0); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

func TestStepUpNeverMovesDown(t *testing.T) {
	t.Parallel()
	for from, want := range map[policy.Mode]policy.Mode{policy.ModeOff: policy.ModeBlastRadius, policy.ModeBlastRadius: policy.ModeAll, policy.ModeAll: policy.ModeAll} {
		if got := policy.StepUp(from); got != want {
			t.Errorf("%s: %s, want %s", from, got, want)
		}
	}
}
