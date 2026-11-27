package flow_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/flow"
)

// TestAllListsEveryStateConstant keeps flow.All in step with the State constants: a new state
// that is not listed would be missing from every group built on it.
func TestAllListsEveryStateConstant(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "flow.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var want []flow.State
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, spec := range g.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "State" {
				continue
			}
			for _, v := range vs.Values {
				lit := v.(*ast.BasicLit)
				want = append(want, flow.State(lit.Value[1:len(lit.Value)-1]))
			}
		}
	}
	if got := flow.All(); !slices.Equal(got, want) {
		t.Fatalf("All() = %v, want the constants in order %v", got, want)
	}
}

// TestStateGroups pins each group to its members: a change here is a change of behavior
// (which tasks the conductor ticks, which take a slot, what the digest and /status show).
func TestStateGroups(t *testing.T) {
	for _, tc := range []struct {
		name string
		keep func(flow.State) bool
		want []string
	}{
		{"driven", flow.State.Driven, []string{"idea", "grilling", "doc_checks", "queued", "building", "checks", "review", "preview", "acceptance", "human_review", "ready", "merging", "rolled_back", "handed_off"}},
		{"takes a slot", flow.State.TakesSlot, []string{"building", "checks", "review", "preview", "acceptance"}},
		{"in progress", flow.State.InProgress, []string{"grilling", "doc_checks", "queued", "building", "checks", "review", "preview", "acceptance", "merging"}},
		{"waits for the owner", flow.State.WaitsForOwner, []string{"spec", "design_review", "human_review", "ready", "escalated"}},
		{"ended", flow.State.Ended, []string{"released", "merged_by_team", "rejected", "canceled"}},
		{"open", flow.Open, []string{"idea", "grilling", "doc_checks", "spec", "design_review", "queued", "blocked", "building", "checks", "review", "preview", "acceptance", "human_review", "ready", "merging", "rolled_back", "handed_off", "escalated"}},
	} {
		if got := flow.Names(tc.keep); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := len(flow.Names(flow.Every)); got != len(flow.All()) {
		t.Errorf("Every keeps %d states, want all %d", got, len(flow.All()))
	}
}

// A slot is taken only by a state the conductor drives, and an ended task waits for nobody.
func TestStateGroupsAgree(t *testing.T) {
	for _, s := range flow.All() {
		if s.TakesSlot() && !s.Driven() {
			t.Errorf("%s takes a slot but is not driven", s)
		}
		if flow.Terminal(s) && !s.Ended() {
			t.Errorf("%s is terminal but not ended", s)
		}
		if s.Ended() && (s.Driven() || s.WaitsForOwner() || s.InProgress()) {
			t.Errorf("%s is ended but still driven, working or waiting", s)
		}
	}
}

func TestUnknownStateIsInNoGroup(t *testing.T) {
	s := flow.State("no_such_state")
	if s.Driven() || s.TakesSlot() || s.InProgress() || s.WaitsForOwner() || s.Ended() {
		t.Fatal("an unknown state is in a group")
	}
}
