package policy

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestDeciders(t *testing.T) {
	t.Parallel()
	var solo config.People
	solo.Owner.Chat, solo.Owner.GitHub, solo.Approvers = "U0OWNER", "example-owner", []string{"owner", "github:example-reviewer"}
	var team config.People
	team.Owner.Chat, team.Owner.GitHub, team.Approvers = "U0OWNER", "example-owner", []string{"github-team:example-org/maintainers"}

	chat := []struct {
		p    config.People
		user string
		want bool
	}{{solo, "U0OWNER", true}, {solo, "U0OTHER", false}, {solo, "", false}, {config.People{}, "", false}}
	for _, tc := range chat {
		if got := CanDecideInChat(tc.p, tc.user); got != tc.want {
			t.Errorf("CanDecideInChat(%q) = %v", tc.user, got)
		}
	}
	gh := []struct {
		p     config.People
		login string
		want  bool
	}{
		{solo, "example-owner", true}, {solo, "example-reviewer", true}, {solo, "stranger", false},
		{team, "example-owner", false}, // in team the owner is not an approver unless listed
		{solo, "", false},
	}
	for _, tc := range gh {
		if got := CanDecideOnGitHub(tc.p, tc.login); got != tc.want {
			t.Errorf("CanDecideOnGitHub(%q) = %v, want %v", tc.login, got, tc.want)
		}
	}
}
