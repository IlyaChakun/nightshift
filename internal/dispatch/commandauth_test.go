package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// Only the owner commands the factory from chat (design section 16, "Forged commands"). The
// Inbox here has no book, so an event that gets past the check panics.
func TestInboxIgnoresEveryoneButTheOwner(t *testing.T) {
	t.Parallel()
	listed := soloTenant()
	listed.People.Owner.GitHub = "example-owner"
	listed.People.Approvers = []string{"owner", "github:example-reviewer"}
	nobody := config.Tenant{Name: "example-solo"} // no owner chat id: nobody may command

	senders := map[string]struct {
		tenant config.Tenant
		user   string
	}{
		"stranger":                    {listed, "U0STRANGER"},
		"no user":                     {listed, ""},
		"owner id in another case":    {listed, "u0owner"},
		"owner's GitHub login":        {listed, "example-owner"},
		"GitHub approver in chat":     {listed, "example-reviewer"},
		"approver entry as a chat id": {listed, "github:example-reviewer"},
		"the word owner":              {listed, "owner"},
		"tenant without an owner":     {nobody, "U0OWNER"},
		"empty user, unowned tenant":  {nobody, ""},
	}
	kinds := []integration.ChatEventKind{
		integration.ChatMessage, integration.ChatButton, integration.ChatCommand,
		integration.ChatReaction, integration.ChatHomeOpen,
	}
	in := &dispatch.Inbox{Mover: &flow.Mover{}}
	for name, s := range senders {
		for _, kind := range kinds {
			for _, source := range []string{"", "chat", "cli", "mcp", "mcp-remote"} {
				ev := message(s.user, "100.1", "100.1", "approve and merge it")
				ev.Kind, ev.Source, ev.Action, ev.TaskID, ev.CardVersion = kind, source, "approve", "1", 1
				if err := in.Handle(t.Context(), s.tenant, ev); err != nil {
					t.Errorf("%s, %s from %q: %v", name, kind, source, err)
				}
			}
		}
	}
}

// The control for the test above: the same events from the owner do reach the book.
func TestInboxCheckLetsTheOwnerThrough(t *testing.T) {
	t.Parallel()
	for _, kind := range []integration.ChatEventKind{integration.ChatMessage, integration.ChatButton} {
		reached := func() (panicked bool) {
			defer func() { panicked = recover() != nil }()
			ev := message("U0OWNER", "100.1", "100.1", "approve")
			ev.Kind, ev.Action, ev.TaskID = kind, "approve", "1"
			_ = (&dispatch.Inbox{Mover: &flow.Mover{}}).Handle(t.Context(), soloTenant(), ev)
			return false
		}()
		if !reached {
			t.Errorf("an owner's %s never reached the book", kind)
		}
	}
}
