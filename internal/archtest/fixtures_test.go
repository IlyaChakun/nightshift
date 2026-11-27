package archtest_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// TestPolicyCatches runs the checker on small synthetic graphs, so a broken checker cannot pass
// the real graph silently.
func TestPolicyCatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pkgs []pkg
		excs []exception
		want []string // substrings, one per expected message; none means it passes
	}{
		{
			name: "allowed edge passes",
			pkgs: []pkg{{Path: "internal/flow", Imports: []string{"internal/book/bookdb"}}, {Path: "internal/book/bookdb"}},
		},
		{
			name: "forbidden plane edge fails",
			pkgs: []pkg{{Path: "internal/memory", Imports: []string{"internal/flow"}}, {Path: "internal/flow"}},
			want: []string{"internal/memory → internal/flow: plane may not import plane (knowledge → control)"},
		},
		{
			name: "control reaches execution only through its ports",
			pkgs: []pkg{{Path: "internal/train", Imports: []string{"internal/harness", "internal/toolchain"}}, {Path: "internal/harness"}, {Path: "internal/toolchain"}},
			want: []string{"internal/train → internal/toolchain"},
		},
		{
			name: "core importing a concrete adapter fails, a fake in a root passes",
			pkgs: []pkg{
				{Path: "internal/flow", Imports: []string{"internal/adapters/chat/slackapp"}},
				{Path: "internal/adapters", Imports: []string{"internal/adapters/chat/fake"}},
				{Path: "internal/adapters/chat/slackapp"}, {Path: "internal/adapters/chat/fake"},
			},
			want: []string{"internal/flow → internal/adapters/chat/slackapp: only a composition root"},
		},
		{
			name: "worker reaching the book transitively fails with its path",
			pkgs: []pkg{
				{Path: "internal/runner", Imports: []string{"internal/harness"}},
				{Path: "internal/harness", Imports: []string{"internal/book/bookdb"}},
				{Path: "internal/book/bookdb", Imports: []string{"github.com/jackc/pgx/v5"}},
			},
			want: []string{
				"internal/harness → internal/book/bookdb: plane may not import plane",
				"internal/runner → internal/book: the worker side reaches the book (internal/runner → internal/harness → internal/book/bookdb)",
				"internal/runner → github.com/jackc/pgx: the worker side reaches the book",
			},
		},
		{
			name: "unknown package fails even with an exception",
			pkgs: []pkg{{Path: "internal/brandnew"}},
			excs: []exception{{From: "internal/brandnew", To: "x", Item: "A0"}},
			want: []string{"internal/brandnew has no plane", "exception internal/brandnew → x (A0, ) matches no violation"},
		},
		{
			name: "an exception excuses its exact edge only",
			pkgs: []pkg{{Path: "internal/memory", Imports: []string{"internal/flow", "internal/train"}}, {Path: "internal/flow"}, {Path: "internal/train"}},
			excs: []exception{{From: "internal/memory", To: "internal/flow", Item: "A0"}},
			want: []string{"internal/memory → internal/train"},
		},
		{
			name: "vendor SDK outside its owner fails",
			pkgs: []pkg{{Path: "internal/dispatch", Imports: []string{"example.com/vendor/sdk"}}, {Path: "internal/adapters/chat/x", Imports: []string{"example.com/vendor/sdk"}}},
			want: []string{"internal/dispatch → example.com/vendor/sdk: vendor SDK outside its owners", "internal/adapters/chat/x has no plane"},
		},
		{
			name: "an unused exception fails",
			pkgs: []pkg{{Path: "internal/flow"}},
			excs: []exception{{From: "internal/flow", To: "internal/runner", Item: "A4", Why: "gone"}},
			want: []string{"exception internal/flow → internal/runner (A4, gone) matches no violation"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := enforce(tt.pkgs, tt.excs, []string{"example.com/vendor"})
			if len(got) != len(tt.want) {
				t.Fatalf("got %d messages, want %d:\n%s", len(got), len(tt.want), strings.Join(got, "\n"))
			}
			for _, w := range tt.want {
				if !slices.ContainsFunc(got, func(m string) bool { return strings.Contains(m, w) }) {
					t.Errorf("no message contains %q:\n%s", w, strings.Join(got, "\n"))
				}
			}
		})
	}
}

// TestEveryExceptionNamesAnOpenItem keeps exceptions tied to an open item of the plan, so an
// exception cannot outlive the work that was meant to remove it.
func TestEveryExceptionNamesAnOpenItem(t *testing.T) {
	t.Parallel()
	plan, err := os.ReadFile("../../docs/plan/architecture/README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range exceptions {
		if e.Why == "" || !strings.Contains(string(plan), "- [ ] "+e.Item+". ") {
			t.Errorf("exception %s → %s needs a reason and an open item (- [ ] %s.) in docs/plan/architecture/README.md", e.From, e.To, e.Item)
		}
	}
}
