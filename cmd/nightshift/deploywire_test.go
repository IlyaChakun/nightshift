package main

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestSpecFor(t *testing.T) {
	t.Parallel()
	doc := `capabilities:
  merge_train: true
  preview: {kind: compose-local, file: deploy/compose.yaml, entry: web, limits: {cpus: 1, memory: 512m}}
  release: {kind: compose-local}
  smoke: ./smoke.sh
`
	caps, err := config.ParseCapabilities([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op   string
		want bool
	}{{"preview", true}, {"release", true}} {
		spec, ok := specFor(caps, tc.op, "main")
		if ok != tc.want || spec.File != "deploy/compose.yaml" || spec.Entry != "web" || spec.Memory != "512m" || spec.Smoke != "./smoke.sh" || spec.Base != "main" {
			t.Errorf("%s: spec = %+v, %v", tc.op, spec, ok)
		}
	}
	// A script or external adapter is not compose-local: the task is released at the merge.
	script, _ := config.ParseCapabilities([]byte("capabilities:\n  merge_train: true\n  release: make deploy\n  preview: null\n"))
	if _, ok := specFor(script, "release", "main"); ok {
		t.Error("a script release must not go to the compose-local deployer")
	}
	if _, ok := specFor(script, "preview", "main"); ok {
		t.Error("no preview capability, no preview")
	}
}
