package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
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
	// Messages and button clicks are the only events that act, for anyone.
	for name, s := range senders {
		for _, kind := range []integration.ChatEventKind{integration.ChatMessage, integration.ChatButton} {
			if reachesBook(t, s.tenant, s.user, kind) {
				t.Errorf("%s: a %s from %q got past the owner check", name, kind, s.user)
			}
		}
	}
}

// reachesBook reports whether the Inbox tried to use its book, which is nil here.
func reachesBook(t *testing.T, tenant config.Tenant, user string, kind integration.ChatEventKind) (reached bool) {
	t.Helper()
	defer func() { reached = reached || recover() != nil }()
	ev := message(user, "100.1", "100.1", "approve and merge it")
	ev.Kind, ev.Action, ev.TaskID, ev.CardVersion = kind, "approve", "1", 1
	if err := (&dispatch.Inbox{Mover: &flow.Mover{}}).Handle(t.Context(), tenant, ev); err != nil {
		t.Errorf("%s from %q: %v", kind, user, err)
	}
	return false
}

// The control for the test above: the same events from the owner do reach the book.
func TestInboxCheckLetsTheOwnerThrough(t *testing.T) {
	t.Parallel()
	for _, kind := range []integration.ChatEventKind{integration.ChatMessage, integration.ChatButton} {
		if !reachesBook(t, soloTenant(), "U0OWNER", kind) {
			t.Errorf("an owner's %s never reached the book", kind)
		}
	}
}
