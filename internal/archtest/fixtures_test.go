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
			pkgs: []pkg{{Path: "internal/knowledge/memory", Imports: []string{"internal/flow"}}, {Path: "internal/flow"}},
			want: []string{"internal/knowledge/memory → internal/flow: plane may not import plane (knowledge → control)"},
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
				{Path: "internal/runner", Imports: []string{"internal/assurance/check"}},
				{Path: "internal/assurance/check", Imports: []string{"internal/book/bookdb"}},
				{Path: "internal/book/bookdb", Imports: []string{"github.com/jackc/pgx/v5"}},
			},
			want: []string{
				"internal/assurance/check → internal/book/bookdb: plane may not import plane",
				"internal/assurance/check → internal/book/bookdb: the worker side reaches the book (internal/runner → internal/assurance/check)",
			},
		},
		{
			name: "an excused route does not hide a new one",
			pkgs: []pkg{
				{Path: "internal/runner", Imports: []string{"internal/jobsapi", "internal/assurance/check"}},
				{Path: "internal/jobsapi", Imports: []string{"github.com/jackc/pgx/v5"}},
				{Path: "internal/assurance/check", Imports: []string{"internal/assurance/standards"}},
				{Path: "internal/assurance/standards", Imports: []string{"github.com/jackc/pgx/v5"}},
			},
			excs: []exception{{From: "internal/jobsapi", To: "github.com/jackc/pgx/v5", Item: "A4"}},
			want: []string{"internal/assurance/standards → github.com/jackc/pgx/v5: the worker side reaches the book (internal/runner → internal/assurance/check → internal/assurance/standards)"},
		},
		{
			name: "the worker half of Git may not reach the trusted half",
			pkgs: []pkg{{Path: "internal/gitops", Imports: []string{"internal/trustedgit"}}, {Path: "internal/trustedgit", Imports: []string{"internal/gitops"}}},
			want: []string{"internal/gitops → internal/trustedgit: plane may not import plane (execution → control)"},
		},
		{
			name: "every worker root is checked, not only the runner",
			pkgs: []pkg{{Path: "internal/harness/claude", Imports: []string{"github.com/jackc/pgx/v5/pgxpool"}}},
			want: []string{"internal/harness/claude → github.com/jackc/pgx/v5/pgxpool: the worker side reaches the book"},
		},
		{
			name: "unknown package fails even with an exception",
			pkgs: []pkg{{Path: "internal/brandnew"}},
			excs: []exception{{From: "internal/brandnew", To: "x", Item: "A0"}},
			want: []string{"internal/brandnew has no plane", "exception internal/brandnew → x (A0, ) matches no violation"},
		},
		{
			name: "an exception excuses its exact edge only",
			pkgs: []pkg{{Path: "internal/knowledge/memory", Imports: []string{"internal/flow", "internal/train"}}, {Path: "internal/flow"}, {Path: "internal/train"}},
			excs: []exception{{From: "internal/knowledge/memory", To: "internal/flow", Item: "A0"}},
			want: []string{"internal/knowledge/memory → internal/train"},
		},
		{
			name: "vendor SDK outside its owner fails",
			pkgs: []pkg{{Path: "internal/dispatch", Imports: []string{"example.com/vendor/sdk"}}, {Path: "internal/adapters/chat/x", Imports: []string{"example.com/vendor/sdk"}}},
			want: []string{"internal/dispatch → example.com/vendor/sdk: vendor SDK outside its owners", "internal/adapters/chat/x has no plane"},
		},
		{
			name: "observability reads flow but drives nothing",
			pkgs: []pkg{{Path: "internal/observability/summary", Imports: []string{"internal/flow", "internal/dispatch", "internal/runner"}}, {Path: "internal/flow"}, {Path: "internal/dispatch"}, {Path: "internal/runner"}},
			want: []string{"internal/observability/summary → internal/dispatch: plane may not import plane (observability → control)", "internal/observability/summary → internal/runner: plane may not import plane (observability → execution)"},
		},
		{
			name: "control may not import experience",
			pkgs: []pkg{{Path: "internal/dispatch", Imports: []string{"internal/mcpdoor"}}, {Path: "internal/mcpdoor"}},
			want: []string{"internal/dispatch → internal/mcpdoor: plane may not import plane (control → experience)"},
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

// TestPlanesHaveNoBroadTrees keeps "/..." entries to the short list, so a new package is never
// classified silently by a broad tree.
func TestPlanesHaveNoBroadTrees(t *testing.T) {
	t.Parallel()
	for k := range planes {
		if strings.HasSuffix(k, "/...") && !slices.Contains(treesAllowed, k) {
			t.Errorf("planes has the tree %q; list packages one by one, or add it to treesAllowed with a reason", k)
		}
	}
}

// TestDepguardOwners keeps vendorOwners equal to the trees the depguard rule core exempts.
func TestDepguardOwners(t *testing.T) {
	t.Parallel()
	denied, owners := depguardCore(t)
	if len(denied) == 0 {
		t.Fatal("the depguard rule core denies nothing")
	}
	for _, d := range denied {
		if strings.ContainsAny(d, `"' `) {
			t.Errorf("depguard deny %q is not a plain import path", d)
		}
	}
	got, want := slices.Sorted(slices.Values(owners)), slices.Sorted(slices.Values(vendorOwners))
	if !slices.Equal(got, want) {
		t.Errorf("depguard core exempts %v, vendorOwners is %v; change both together", got, want)
	}
}

// TestFolderMatchesPlane proves that a package in a plane folder must carry that plane.
func TestFolderMatchesPlane(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		from plane
		want string
	}{
		{"internal/knowledge/search", knowledge, ""},
		{"internal/knowledge/search", control, "is in the knowledge folder but classified as control"},
		{"internal/customization/stack", assurance, "is in the customization folder but classified as assurance"},
		{"internal/flow", control, ""},
		{"internal/adapters/github", control, ""},
		{"cmd/nightshift", root, ""},
	}
	for _, tt := range tests {
		got := folderProblem(tt.path, tt.from)
		if (tt.want == "") != (got == "") || !strings.Contains(got, tt.want) {
			t.Errorf("folderProblem(%q, %s) = %q, want %q", tt.path, tt.from, got, tt.want)
		}
	}
}

// TestMissingWorkerRoot proves that a moved worker root fails instead of being skipped.
func TestMissingWorkerRoot(t *testing.T) {
	t.Parallel()
	mod := map[string]pkg{"internal/runner": {Path: "internal/runner"}}
	got := missingRoots([]string{"runner", "config"}, mod)
	if len(got) != 1 || !strings.Contains(got[0], "internal/config is not a package") {
		t.Errorf("missingRoots = %q, want one message for internal/config", got)
	}
}
