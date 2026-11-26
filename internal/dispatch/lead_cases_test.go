package dispatch

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/config"
)

type leadCase struct {
	ID       string   `yaml:"id"`
	Owner    string   `yaml:"owner"`
	Must     []string `yaml:"must"`
	Good     string   `yaml:"good"`
	Bad      string   `yaml:"bad"`
	BadWords int      `yaml:"bad_words"`
	BadError string   `yaml:"bad_error"`
	Expect   string   `yaml:"expect"`
}

// TestLeadCasesPinTheChatRules replays the frozen cases without a model: the reference reply
// passes the check and the bad one fails for the stated reason.
func TestLeadCasesPinTheChatRules(t *testing.T) {
	raw, err := os.ReadFile("../../evals/lead/cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cases []leadCase
	if err := yaml.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
		t.Fatalf("cases: %v", err)
	}
	v := config.NewValidator()
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) { checkLeadCase(t, v, c) })
	}
}

// checkLeadCase: the reference reply passes the check and its own `expect`; the bad one fails for
// the stated reason.
func checkLeadCase(t *testing.T, v *config.Validator, c leadCase) {
	t.Helper()
	if c.Owner == "" || len(c.Must) == 0 {
		t.Fatal("a case needs an owner message and a `must` list")
	}
	good, err := CheckLeadReply(v, []byte(c.Good))
	if err != nil {
		t.Fatalf("the reference reply fails: %v", err)
	}
	if err := CheckExpect(c.Expect, good); err != nil {
		t.Fatalf("the reference reply cannot satisfy its own expect: %v", err)
	}
	bad := c.Bad
	if c.BadWords > 0 {
		bad = `{"type":"blocker_or_summary","text":"` + strings.Repeat("word ", c.BadWords) + `"}`
	}
	if _, err := CheckLeadReply(v, []byte(bad)); err == nil || !strings.Contains(err.Error(), c.BadError) {
		t.Fatalf("the bad reply gave %v, want %q", err, c.BadError)
	}
}

// TestLeadRoleExamplesPassTheCheck keeps the example replies in roles/lead.md valid: an
// example the check would reject teaches Lead to fail.
func TestLeadRoleExamplesPassTheCheck(t *testing.T) {
	raw, err := os.ReadFile("../../roles/lead.md")
	if err != nil {
		t.Fatal(err)
	}
	examples := regexp.MustCompile("(?m)^`(\\{\"type\".*\\})`$").FindAllStringSubmatch(string(raw), -1)
	if len(examples) < 2 {
		t.Fatalf("found %d example replies in roles/lead.md", len(examples))
	}
	v := config.NewValidator()
	types := map[string]bool{}
	for _, m := range examples {
		r, err := CheckLeadReply(v, []byte(m[1]))
		if err != nil {
			t.Errorf("example %.60s: %v", m[1], err)
		}
		types[r.Type] = true
	}
	// The model copies the shapes it is shown: every type Lead must tell apart needs an example, and
	// a reply that needs a choice must show options and a recommendation.
	for _, want := range []string{"question", "decision_needed", "blocker_or_summary"} {
		if !types[want] {
			t.Errorf("roles/lead.md has no example of a %s reply", want)
		}
	}
}

// TestLeadRoleStatesTheTypeRules keeps the prose rule and the checker in step: the role file must
// tell Lead that a question carries options and a recommendation, which CheckLeadReply enforces.
func TestLeadRoleStatesTheTypeRules(t *testing.T) {
	raw, err := os.ReadFile("../../roles/lead.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"Never a question without options", "`recommended`", "Only here is `confidence: low` accepted"} {
		if !strings.Contains(text, want) {
			t.Errorf("roles/lead.md no longer says %q", want)
		}
	}
	// A reply has one `options` array and one `recommended`, so two questions cannot fit in it.
	for _, bad := range []string{"one or two questions", "two questions"} {
		if strings.Contains(text, bad) {
			t.Errorf("roles/lead.md says %q, but a reply carries the options of one question", bad)
		}
	}
	v := config.NewValidator()
	if _, err := CheckLeadReply(v, []byte(`{"type":"question","understood":"The owner wants the change described in the task.","text":"Which?"}`)); err == nil {
		t.Error("the checker accepts a question without options: the role file's rule is stale")
	}
}
