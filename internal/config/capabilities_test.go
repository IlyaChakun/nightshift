package config

import (
	"os"
	"testing"
)

func TestParseCapabilitiesOfTheSyntheticTenants(t *testing.T) {
	t.Parallel()
	solo, err := os.ReadFile("../../testdata/tenants/example-solo/.nightshift/project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCapabilities(solo)
	if err != nil {
		t.Fatal(err)
	}
	if !c.MergeTrain || c.Preview.Kind != "script" || c.Preview.Up != "./scripts/preview-up.sh" ||
		c.Release.Kind != "script" || c.Release.Command != "./scripts/release.sh" || c.Smoke.Command != "./scripts/smoke-main-path.sh" {
		t.Errorf("solo = %+v preview %+v release %+v", c, c.Preview, c.Release)
	}
	team, err := os.ReadFile("../../testdata/tenants/example-team/.nightshift/project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c, err = ParseCapabilities(team); err != nil {
		t.Fatal(err)
	}
	if c.MergeTrain || c.Preview.Kind != "external" || len(c.Preview.Envs) != 1 || c.Release != nil || c.Rollback != nil || c.Smoke != nil {
		t.Errorf("team = %+v preview %+v", c, c.Preview)
	}
}

func TestParseCapabilitiesComposeLocal(t *testing.T) {
	t.Parallel()
	doc := "capabilities:\n  merge_train: true\n" +
		"  preview: { kind: compose-local, file: deploy/compose.yaml, entry: web, limits: { cpus: 2, memory: 2g } }\n" +
		"  release: { kind: compose-local, file: deploy/compose.yaml }\n  rollback: { kind: compose-local }\n  smoke: ./smoke.sh\n"
	c, err := ParseCapabilities([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Preview; p.Kind != "compose-local" || p.File != "deploy/compose.yaml" || p.Entry != "web" || p.CPUs != 2 || p.Memory != "2g" {
		t.Errorf("preview = %+v", p)
	}
	if c.Release.File != "deploy/compose.yaml" || c.Rollback.Kind != "compose-local" || c.Smoke.Command != "./smoke.sh" {
		t.Errorf("caps = %+v", c)
	}
}

func TestParseCapabilitiesRefusesAMissingBlock(t *testing.T) {
	t.Parallel()
	if _, err := ParseCapabilities([]byte("name: p\n")); err == nil {
		t.Error("a project without capabilities was accepted")
	}
}
