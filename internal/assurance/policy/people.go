// Package policy holds the factory's decision rules: who may decide, and later the blast
// radius and the review policy (design section 11).
package policy

import (
	"slices"

	"github.com/ilyachakun/nightshift/internal/config"
)

// CanDecideInChat reports whether a chat user may send commands and press decision buttons.
// Only the tenant's owner can; approvers are named by GitHub ids and decide on GitHub.
func CanDecideInChat(p config.People, chatUser string) bool {
	return chatUser != "" && chatUser == p.Owner.Chat
}

// CanDecideOnGitHub reports whether a GitHub login may approve for the tenant.
// Team approvers (github-team:…, codeowners) are resolved by the GitHub adapter in phase 8.
func CanDecideOnGitHub(p config.People, login string) bool {
	if login == "" {
		return false
	}
	if login == p.Owner.GitHub && slices.Contains(p.Approvers, "owner") {
		return true
	}
	return slices.Contains(p.Approvers, "github:"+login)
}
