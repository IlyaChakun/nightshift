package dispatch_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

type mergeLog struct {
	prs   []integration.MergedPR
	err   error
	calls int
}

func (m *mergeLog) MergedSince(context.Context, string, time.Time) ([]integration.MergedPR, error) {
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
	o := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat}, Mask: dispatch.NewMasker()}
	log := &mergeLog{prs: []integration.MergedPR{{Number: 9, Title: "feat: x", URL: "https://x/9", Reviewed: true}}}
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
	if p := string(chat.sent[0].Payload); !strings.Contains(p, "U_OWNER") || !strings.Contains(p, "#9 feat: x") || !strings.Contains(p, "https://x/9") || !strings.Contains(p, "last 24 h (since 25 Nov 09:01 CET)") {
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
	o := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat}, Mask: dispatch.NewMasker()}
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
	log.err, log.prs = nil, []integration.MergedPR{{Number: 3, Title: "late", URL: "https://x/3"}}
	if step(10*time.Minute) != 1 || log.calls != 2 || strings.Contains(string(chat.sent[0].Payload), "incomplete") {
		t.Fatalf("a complete list goes out: sent %d, calls %d", len(chat.sent), log.calls)
	}

	// A code host that stays down: after two hours the digest goes out, marked incomplete.
	chat2 := &recorder{}
	o2 := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat2}, Mask: dispatch.NewMasker()}
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
