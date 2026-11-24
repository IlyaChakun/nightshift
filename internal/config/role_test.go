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
	if v := roles["verifier"]; !v.ReadOnly() || !v.FreshContext {
		t.Errorf("verifier: %+v", v)
	}
	for _, in := range b.Inputs {
		if in == "hidden_scenarios" {
			t.Error("a Builder must never ask for hidden scenarios")
		}
	}
}

func TestLoadRolesRejects(t *testing.T) {
	const ok = "on: [task.x]\naccess: worktree\noutput_schema: schemas/builder-result.json\nlimits: { minutes: 1, tokens: 1 }\n"
	for name, doc := range map[string]string{
		"name differs from file": "name: other\n" + ok,
		"missing schema file":    "name: r\non: [task.x]\naccess: worktree\noutput_schema: schemas/nope.json\nlimits: { minutes: 1, tokens: 1 }\n",
		"invalid by role.json":   "name: r\n",
	} {
		_, err := config.LoadRoles(fstest.MapFS{"r.yaml": {Data: []byte(doc)}})
		if err == nil || !strings.Contains(err.Error(), "roles/r.yaml") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
