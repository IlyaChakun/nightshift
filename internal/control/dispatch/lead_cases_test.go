package dispatch

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/control/leadreply"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/knowledge/memory"
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
	Keep     []string `yaml:"keep"`
	Drop     []string `yaml:"drop"`
}

// TestLeadCasesPinTheChatRules replays the frozen cases without a model: the reference reply
// passes the check and the bad one fails for the stated reason.
func TestLeadCasesPinTheChatRules(t *testing.T) {
	raw, err := os.ReadFile("../../../evals/lead/cases.yaml")
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
	good, err := leadreply.Check(v, []byte(c.Good))
	if err != nil {
		t.Fatalf("the reference reply fails: %v", err)
	}
	if err := leadreply.CheckExpect(c.Expect, good); err != nil {
		t.Fatalf("the reference reply cannot satisfy its own expect: %v", err)
	}
	if _, out := splitObservable([]byte(c.Good)); len(out) > 0 {
		t.Fatalf("the reference reply has hidden scenarios a browser cannot check: %v", out)
	}
	correction := len(c.Keep)+len(c.Drop) > 0
	if correction {
		if err := leadreply.CheckCorrection([]byte(c.Good), c.Keep, c.Drop); err != nil {
			t.Fatalf("the reference reply fails its own correction check: %v", err)
		}
		// The bad reply of a correction case is well-formed: only the correction check can tell it.
		if _, err := leadreply.Check(v, []byte(c.Bad)); err != nil {
			t.Fatalf("the bad reply of a correction case must pass the chat rules: %v", err)
		}
		if err := leadreply.CheckCorrection([]byte(c.Bad), c.Keep, c.Drop); err == nil || !strings.Contains(err.Error(), c.BadError) {
			t.Fatalf("the bad reply gave %v, want %q", err, c.BadError)
		}
		return
	}
	bad := c.Bad
	if c.BadWords > 0 {
		bad = `{"type":"blocker_or_summary","text":"` + strings.Repeat("word ", c.BadWords) + `"}`
	}
	r, err := leadreply.Check(v, []byte(bad))
	if err == nil && c.Expect != "" {
		err = leadreply.CheckExpect(c.Expect, r) // a well-formed reply that does the wrong thing fails its `expect`
	}
	if _, out := splitObservable([]byte(bad)); err == nil && len(out) > 0 {
		err = errNotInBrowser(out) // the conductor sends it back (checkObservable)
	}
	if err == nil || !strings.Contains(err.Error(), c.BadError) {
		t.Fatalf("the bad reply gave %v, want %q", err, c.BadError)
	}
}

// TestLeadRoleExamplesPassTheCheck keeps the example replies in roles/lead.md valid: an
// example the check would reject teaches Lead to fail.
func TestLeadRoleExamplesPassTheCheck(t *testing.T) {
	raw, err := os.ReadFile("../../../roles/lead.md")
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
		r, err := leadreply.Check(v, []byte(m[1]))
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
// tell Lead that a question carries options and a recommendation, which leadreply.Check enforces.
func TestLeadRoleStatesTheTypeRules(t *testing.T) {
	raw, err := os.ReadFile("../../../roles/lead.md")
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
	if _, err := leadreply.Check(v, []byte(`{"type":"question","understood":"The owner wants the change described in the task.","text":"Which?"}`)); err == nil {
		t.Error("the checker accepts a question without options: the role file's rule is stale")
	}
}

var (
	sentenceEnd = regexp.MustCompile(`\. |\n`)
	denial      = regexp.MustCompile(`(?i)not (yet )?available|unavailable|not built`)
)

// deniedInputs lists the sentences of a role text that say the book search or the project memory
// is missing. The conductor puts both into Lead's prompt, so such a sentence is false.
func deniedInputs(role string) []string {
	var out []string
	for _, sentence := range sentenceEnd.Split(role, -1) {
		low := strings.ToLower(sentence)
		if denial.MatchString(sentence) && (strings.Contains(low, "book search") || strings.Contains(low, "project memory")) {
			out = append(out, sentence)
		}
	}
	return out
}

// TestLeadTextsNameThePromptParts keeps the role and its skills true to the prompt the conductor
// builds. `roles/lead.md` once said the book search and the project memory were not available
// while the prompt carried both, so Lead was told to ignore what it had been given.
func TestLeadTextsNameThePromptParts(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(path) //nolint:gosec // files of this repository
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	role := read("../../../roles/lead.md")
	for _, heading := range []string{similarHeading, memory.PromptHeading} {
		if !strings.Contains(role, `"`+heading+`"`) {
			t.Errorf("roles/lead.md does not name the prompt part %q", heading)
		}
	}
	for _, denied := range deniedInputs(role) {
		t.Errorf("roles/lead.md denies an input that the conductor puts into the prompt: %s", denied)
	}
	// The skills that send Lead to the book search quote the heading; a rename must reach them.
	for _, skill := range []string{"challenge", "grilling", "build-vs-buy", "research-options", "support"} {
		if !strings.Contains(read("../../../skills/own/"+skill+"/SKILL.md"), `"`+similarHeading+`"`) {
			t.Errorf("skills/own/%s/SKILL.md does not name the prompt part %q", skill, similarHeading)
		}
	}
}
