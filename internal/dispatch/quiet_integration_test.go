package dispatch_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// TestOutboxParksOwnerCardsInQuietHours covers design section 17: a card that needs the owner waits
// for the morning, without being counted as a failed attempt, and goes out when the window ends.
func TestOutboxParksOwnerCardsInQuietHours(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx := t.Context()
	var tenant config.Tenant
	tenant.Name = "example-solo"
	tenant.Schedule = config.Schedule{Timezone: "Europe/Zurich", QuietHours: "22:00-09:00"}
	night := time.Date(2026, 11, 26, 22, 30, 0, 0, time.UTC).Add(-time.Hour) // 22:30 in Zurich
	clock := night
	chat := &recorder{}
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat}, Mask: dispatch.NewMasker("x-secret-value"),
		Now:  func() time.Time { return clock },
		Hold: dispatch.QuietHold(map[string]config.Tenant{tenant.Name: tenant}, slog.Default()),
	}
	q := bookdb.New(pool)
	enqueue := func(key, kind string) {
		t.Helper()
		if err := o.Enqueue(ctx, q, tenant.Name, 0, "chat", key, []byte(`{"kind":"`+kind+`","text":"t"}`)); err != nil {
			t.Fatal(err)
		}
	}
	enqueue("decision", "decision_needed")
	enqueue("ready", "ready_for_acceptance")
	enqueue("answer", "question")
	enqueue("incident", "blocker_or_summary")

	if n, err := o.Flush(ctx); err != nil || n != 2 {
		t.Fatalf("at night: sent %d, %v; want the question and the blocker only", n, err)
	}
	var attempts int32
	var next time.Time
	var status string
	row := db.Owner.QueryRowContext(ctx, `SELECT status, attempts, next_attempt_at FROM book.outbox WHERE idempotency_key = 'decision'`)
	if err := row.Scan(&status, &attempts, &next); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC); status != "pending" || attempts != 0 || !next.Equal(want) {
		t.Fatalf("held card: %s, %d attempts, until %s; want pending, 0, %s", status, attempts, next, want)
	}

	// Morning: the window is over and the parking time has passed.
	clock = time.Date(2026, 11, 27, 8, 0, 1, 0, time.UTC)
	if _, err := db.Owner.ExecContext(ctx, `UPDATE book.outbox SET next_attempt_at = now() WHERE status = 'pending'`); err != nil {
		t.Fatal(err)
	}
	if n, err := o.Flush(ctx); err != nil || n != 2 {
		t.Fatalf("in the morning: sent %d, %v; want both owner cards", n, err)
	}
}

func TestQuietHoldKeepsWhatItCannotRead(t *testing.T) {
	t.Parallel()
	bad := config.Tenant{}
	bad.Schedule = config.Schedule{QuietHours: "garbage"}
	hold := dispatch.QuietHold(map[string]config.Tenant{"t": bad}, slog.Default())
	row := bookdb.BookOutbox{Tenant: "t", Target: "chat", Payload: []byte(`{"kind":"decision_needed"}`)}
	if _, held := hold(row, time.Now()); held {
		t.Error("a broken quiet_hours must not hold a card")
	}
	row.Tenant = "unknown"
	if _, held := hold(row, time.Now()); held {
		t.Error("an unknown tenant must not hold a card")
	}
}
