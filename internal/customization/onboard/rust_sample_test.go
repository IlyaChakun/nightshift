package onboard_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/customization/onboard"
	"github.com/ilyachakun/nightshift/internal/customization/stack"
)

// TestARustProjectWithoutAPackPassesTheDoctor is the phase 4 Done check for any language: a
// project in a language with no stack pack connects with its own toolchain and verify command.
func TestARustProjectWithoutAPackPassesTheDoctor(t *testing.T) {
	t.Parallel()
	repo := os.DirFS("../../../testdata/projects/rust-sample")
	packs, err := stack.Load(os.DirFS("../../../stacks"))
	if err != nil {
		t.Fatal(err)
	}
	found := onboard.Discover(repo, "rust-sample", packs)
	if len(found.Stacks) != 0 || len(found.Unchecked) != 1 || found.Unchecked[0] != "Cargo.toml" {
		t.Fatalf("discovery: %+v", found)
	}
	have := map[string]bool{"mise": true, "make": true, "cargo": true}
	d := onboard.Doctor{
		Repo: repo, Validator: config.NewValidator(), Packs: packs,
		Look: func(n string) error {
			if have[n] {
				return nil
			}
			return errors.New("not found")
		},
		Secret: func(string) error { return nil },
	}
	rs := d.Run()
	if onboard.Failed(rs) {
		t.Fatalf("a project without a pack must connect:\n%s", onboard.Format(rs))
	}
	if r, ok := byName(rs, "stack rust"); !ok || r.Status != onboard.Warn || !strings.Contains(r.Fix, ".nightshift/stacks/rust/pack.yaml") {
		t.Fatalf("the doctor must name the missing pack: %+v\n%s", r, onboard.Format(rs))
	}
}
