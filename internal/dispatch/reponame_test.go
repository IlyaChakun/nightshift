package dispatch

import "testing"

// A task from chat must name its repository as git mirrors and pull requests expect it.
func TestRepoName(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"github.com/example-owner/example-solo": "example-owner/example-solo",
		"example-owner/example-solo":            "example-owner/example-solo",
	} {
		if got := repoName(in); got != want {
			t.Errorf("repoName(%q) = %q, want %q", in, got, want)
		}
	}
}
