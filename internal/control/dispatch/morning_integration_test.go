package dispatch_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/observability/summary"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

type mergeLog struct {
	prs   []summary.MergedPR
	err   error
	calls int
}

// recorder is a chat sender that keeps what it was given.
type recorder struct {
	mu   sync.Mutex
	sent []outbox.Message
	fail error
}

func (r *recorder) Send(_ context.Context, m outbox.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.sent = append(r.sent, m)
	return nil
}

func (m *mergeLog) MergedSince(context.Context, string, time.Time) ([]summary.MergedPR, error) {
	m.calls++
	return m.prs, m.err
}

func canDM(config.Tenant) bool { return true }

func morningTenant() config.Tenant {
	t := config.Tenant{Name: "example-solo", Repos: []string{"github.com/example/app"}}
	t.People.Owner.Chat = "U_OWNER"
	t.Schedule.Timezone = "Europe/Berlin"
	t.Schedule.MorningSummary = "09:00"
	return t
}

func TestMorningDigestGoesOutOncePerLocalDayAfterTheTime(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx := t.Context()
	chat := &recorder{}
	o := &outbox.Outbox{Pool: pool, Senders: map[string]outbox.Sender{"chat": chat}, Mask: mask.NewMasker()}
	log := &mergeLog{prs: []summary.MergedPR{{Number: 9, Title: "feat: x", URL: "https://x/9", Reviewed: true}}}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	at := time.Date(2026, 11, 26, 8, 59, 0, 0, berlin) // one minute before the time
	m := &dispatch.Morning{Pool: pool, Outbox: o, Merges: log, CanMessageOwner: canDM, Now: func() time.Time { return at }}
	flush := func() int {
		t.Helper()
		if _, err := o.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		return len(chat.sent)
	}

	if err := m.Run(ctx, morningTenant()); err != nil || flush() != 0 || log.calls != 0 {
		t.Fatalf("before 09:00 nothing is built or sent: sent %d, calls %d, err %v", len(chat.sent), log.calls, err)
	}
	at = at.Add(2 * time.Minute)
	if err := m.Run(ctx, morningTenant()); err != nil || flush() != 1 {
		t.Fatalf("after 09:00 one message: %d %v", len(chat.sent), err)
	}
	if p := string(chat.sent[0].Payload); !strings.Contains(p, "U_OWNER") || !strings.Contains(p, `New (1):\n• x`) || !strings.Contains(p, `• x https://x/9`) || !strings.Contains(p, "last 24 h (since 25 Nov 09:01 CET)") {
		t.Fatalf("payload: %s", p)
	}
	// Later the same day, and a restarted service (a new Morning): still one message.
	at = at.Add(3 * time.Hour)
	if err := m.Run(ctx, morningTenant()); err != nil {
		t.Fatal(err)
	}
	again := &dispatch.Morning{Pool: pool, Outbox: o, Merges: log, CanMessageOwner: canDM, Now: func() time.Time { return at }}
	if err := again.Run(ctx, morningTenant()); err != nil || flush() != 1 {
		t.Fatalf("a repeat the same day sent %d messages", len(chat.sent))
	}
	// The next day it goes out again.
	at = at.Add(24 * time.Hour)
	if err := m.Run(ctx, morningTenant()); err != nil || flush() != 2 {
		t.Fatalf("next day: %d messages, %v", len(chat.sent), err)
	}
}

func TestMorningDigestWaitsForACompleteMergeListThenGoesOutMarked(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx := t.Context()
	chat := &recorder{}
	o := &outbox.Outbox{Pool: pool, Senders: map[string]outbox.Sender{"chat": chat}, Mask: mask.NewMasker()}
	log := &mergeLog{err: errors.New("503")}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	at := time.Date(2026, 11, 26, 9, 1, 0, 0, berlin)
	m := &dispatch.Morning{Pool: pool, Outbox: o, Merges: log, CanMessageOwner: canDM, Now: func() time.Time { return at }}
	step := func(d time.Duration) int {
		t.Helper()
		at = at.Add(d)
		if err := m.Run(ctx, morningTenant()); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		return len(chat.sent)
	}
	if step(0) != 0 || log.calls != 1 {
		t.Fatalf("a failing list holds the digest: sent %d, calls %d", len(chat.sent), log.calls)
	}
	if step(time.Minute) != 0 || log.calls != 1 {
		t.Fatalf("the retry waits 10 minutes: calls %d", log.calls)
	}
	log.err, log.prs = nil, []summary.MergedPR{{Number: 3, Title: "late", URL: "https://x/3"}}
	if step(10*time.Minute) != 1 || log.calls != 2 || strings.Contains(string(chat.sent[0].Payload), "incomplete") {
		t.Fatalf("a complete list goes out: sent %d, calls %d", len(chat.sent), log.calls)
	}

	// A code host that stays down: after two hours the digest goes out, marked incomplete.
	chat2 := &recorder{}
	o2 := &outbox.Outbox{Pool: pool, Senders: map[string]outbox.Sender{"chat": chat2}, Mask: mask.NewMasker()}
	down := &mergeLog{err: errors.New("503")}
	at2 := time.Date(2026, 11, 27, 9, 1, 0, 0, berlin)
	m2 := &dispatch.Morning{Pool: pool, Outbox: o2, Merges: down, CanMessageOwner: canDM, Now: func() time.Time { return at2 }}
	for range 14 { // 09:01 to 11:11
		if err := m2.Run(ctx, morningTenant()); err != nil {
			t.Fatal(err)
		}
		at2 = at2.Add(10 * time.Minute)
	}
	if _, err := o2.Flush(ctx); err != nil || len(chat2.sent) != 1 || !strings.Contains(string(chat2.sent[0].Payload), "incomplete") {
		t.Fatalf("after two hours: %d messages, %v", len(chat2.sent), err)
	}
}

func TestAResentDigestGoesOutEvenWhenTheDailyOneDid(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx := t.Context()
	chat := &recorder{}
	o := &outbox.Outbox{Pool: pool, Senders: map[string]outbox.Sender{"chat": chat}, Mask: mask.NewMasker()}
	m := &dispatch.Morning{Pool: pool, Outbox: o}
	tenant := morningTenant()
	d := summary.Digest{Summary: summary.Summary{Since: time.Now().Add(-24 * time.Hour), Until: time.Now()}}
	queue := func(key string) int {
		t.Helper()
		if err := m.Queue(ctx, tenant, d, key); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		return len(chat.sent)
	}
	if n := queue("digest:example-solo:2026-11-26"); n != 1 {
		t.Fatalf("the daily digest: %d messages", n)
	}
	if n := queue("digest:example-solo:2026-11-26"); n != 1 {
		t.Fatalf("the same key sends once: %d messages", n)
	}
	if n := queue("digest-resend:example-solo:2026-11-26T10:30:00Z"); n != 2 {
		t.Fatalf("a fresh key sends again: %d messages", n)
	}
	if p := string(chat.sent[1].Payload); !strings.Contains(p, "U_OWNER") || !strings.Contains(p, "last 24 h") {
		t.Fatalf("payload: %s", p)
	}
	tenant.People.Owner.Chat = ""
	if err := m.Queue(ctx, tenant, d, "digest-resend:none"); err == nil {
		t.Fatal("a tenant without an owner chat must be refused")
	}
}
