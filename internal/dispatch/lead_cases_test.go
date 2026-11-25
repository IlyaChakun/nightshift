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
		t.Run(c.ID, func(t *testing.T) {
			if c.Owner == "" || len(c.Must) == 0 {
				t.Fatal("a case needs an owner message and a `must` list")
			}
			if _, err := CheckLeadReply(v, []byte(c.Good)); err != nil {
				t.Fatalf("the reference reply fails: %v", err)
			}
			bad := c.Bad
			if c.BadWords > 0 {
				bad = `{"type":"blocker_or_summary","text":"` + strings.Repeat("word ", c.BadWords) + `"}`
			}
			if _, err := CheckLeadReply(v, []byte(bad)); err == nil || !strings.Contains(err.Error(), c.BadError) {
				t.Fatalf("the bad reply gave %v, want %q", err, c.BadError)
			}
		})
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
	for _, m := range examples {
		if _, err := CheckLeadReply(v, []byte(m[1])); err != nil {
			t.Errorf("example %.60s: %v", m[1], err)
		}
	}
}
