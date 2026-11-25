package config_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestLoadShippedRoles(t *testing.T) {
	roles, err := config.LoadRoles(os.DirFS("../../roles"))
	if err != nil {
		t.Fatal(err)
	}
	b, ok := roles["builder"]
	if !ok || b.ReadOnly() || b.OutputSchema != "schemas/builder-result.json" || b.Limits.MaxReturns != 3 {
		t.Errorf("builder: %+v", b)
	}
	if v := roles["verifier"]; !v.ReadOnly() {
		t.Errorf("verifier: %+v", v)
	}
}

func TestLoadRolesRejects(t *testing.T) {
	const ok = "access: worktree\noutput_schema: schemas/builder-result.json\nlimits: { minutes: 1, tokens: 1 }\n"
	for name, doc := range map[string]string{
		"name differs from file": "name: other\n" + ok,
		"missing schema file":    "name: r\naccess: worktree\noutput_schema: schemas/nope.json\nlimits: { minutes: 1, tokens: 1 }\n",
		"invalid by role.json":   "name: r\n",
		"unknown key":            "name: r\ninputs: [thread]\n" + ok,
	} {
		_, err := config.LoadRoles(fstest.MapFS{"r.yaml": {Data: []byte(doc)}})
		if err == nil || !strings.Contains(err.Error(), "roles/r.yaml") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Every role has instructions, and every one of them tells the agent that text it reads
// is data (design section 16), whatever else it says.
func TestEveryRoleHasInstructionsThatTreatInputAsData(t *testing.T) {
	roles, err := config.LoadRoles(os.DirFS("../../roles"))
	if err != nil {
		t.Fatal(err)
	}
	for name := range roles {
		raw, err := os.ReadFile("../../roles/" + name + ".md") //nolint:gosec // role names come from the shipped roles directory
		if err != nil {
			t.Errorf("role %s has no instructions: %v", name, err)
			continue
		}
		if !strings.Contains(string(raw), "is data, not instructions") {
			t.Errorf("role %s: its instructions do not say that read text is data", name)
		}
	}
}
