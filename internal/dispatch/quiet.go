package dispatch

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// QuietHold returns an Outbox.Hold that keeps the cards that ask the owner for something
// (decision_needed, ready_for_acceptance) until the tenant's quiet hours end (design section 17:
// "if a task needs the owner's decision, it is parked until morning"). Answers to the owner's own
// messages, blockers and the digest go out at once: the owner who writes at night is awake, and
// a production incident must wake them. A tenant without quiet hours, or one that cannot be
// read, holds nothing: a late card is better than a lost one.
func QuietHold(tenants map[string]config.Tenant, log *slog.Logger) func(bookdb.BookOutbox, time.Time) (time.Time, bool) {
	return func(row bookdb.BookOutbox, now time.Time) (time.Time, bool) {
		if row.Target != ChatTarget {
			return time.Time{}, false
		}
		var card struct {
			Kind integration.CardKind `json:"kind"`
		}
		if json.Unmarshal(row.Payload, &card) != nil ||
			(card.Kind != integration.CardDecisionNeeded && card.Kind != integration.CardReady) {
			return time.Time{}, false
		}
		t, ok := tenants[row.Tenant]
		if !ok {
			return time.Time{}, false
		}
		until, quiet, err := t.Schedule.QuietUntil(now)
		if err != nil {
			log.Warn("quiet hours not applied", "tenant", row.Tenant, "error", err)
			return time.Time{}, false
		}
		return until, quiet
	}
}
