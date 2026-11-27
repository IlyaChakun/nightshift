package dispatch

import (
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// The first live run sent "owner" to GitHub as a login.
func TestReviewersAreLogins(t *testing.T) {
	t.Parallel()
	var tenant config.Tenant
	tenant.People.Owner.GitHub = "example-owner"
	tenant.People.Approvers = []string{"owner", "github:alice", "github:example-owner", "github-team:org/team", "codeowners"}
	if got := reviewers(tenant); !slices.Equal(got, []string{"example-owner", "alice"}) {
		t.Errorf("reviewers: %v", got)
	}
}
