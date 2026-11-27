package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/observability/summary"
)

// defaultMorning is when the digest goes out when the tenant sets no `schedule.morning_summary`.
const defaultMorning = "09:00"

// Morning sends the morning digest (ADR 0013): once per local day, after the tenant's
// `morning_summary` time, a message to the owner with the summary, every merge and every
// decision of the last 24 hours. The outbox key holds the day, so a restart or a second tick
// cannot send it twice.
type Morning struct {
	Pool   *pgxpool.Pool
	Outbox *Outbox
	Merges summary.MergeLog
	// Health, when set, adds the factory health part (open alerts, idle pull requests, main) to the digest.
	Health summary.RepoHealth
	// Plan, when set, adds the "Plan" block to the digest: the open plan files and how far each is.
	Plan summary.PlanLog
	// CanMessageOwner says whether the tenant's chat adapter can post to the owner without a task
	// thread; the digest is sent only to those tenants.
	CanMessageOwner func(config.Tenant) bool
	Now             func() time.Time
	Log             *slog.Logger

	mu    sync.Mutex
	done  map[string]string    // tenant -> local date already queued
	tried map[string]time.Time // tenant -> last attempt whose merge list was incomplete
}

const (
	// retryEvery and giveUpAfter bound the wait for a complete merge list: a code host that
	// fails at 09:00 is asked again, and after two hours the digest goes out marked incomplete.
	retryEvery  = 10 * time.Minute
	giveUpAfter = 2 * time.Hour
)

// Run queues the digest of one tenant when it is due. It does nothing before the time, for a
// tenant whose chat adapter cannot message the owner without a task, or when already queued today.
func (m *Morning) Run(ctx context.Context, t config.Tenant) error {
	if m.CanMessageOwner == nil || !m.CanMessageOwner(t) || t.People.Owner.Chat == "" {
		return nil
	}
	loc, err := time.LoadLocation(orDefault(t.Schedule.Timezone, "UTC"))
	if err != nil {
		return fmt.Errorf("morning: timezone of %s: %w", t.Name, err)
	}
	clock, err := time.Parse("15:04", orDefault(t.Schedule.MorningSummary, defaultMorning))
	if err != nil {
		return fmt.Errorf("morning: morning_summary of %s: %w", t.Name, err)
	}
	now := m.now().In(loc)
	if now.Hour()*60+now.Minute() < clock.Hour()*60+clock.Minute() {
		return nil
	}
	day := now.Format(time.DateOnly)
	m.mu.Lock()
	if m.done == nil {
		m.done, m.tried = map[string]string{}, map[string]time.Time{}
	}
	last, retrying := m.tried[t.Name]
	if m.done[t.Name] == day || (retrying && now.Sub(last) < retryEvery) {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	repos := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		repos = append(repos, repoName(r))
	}
	d, err := summary.BuildDigest(ctx, m.Pool, m.Merges, t.Name, repos, now.Add(-24*time.Hour), now)
	if err != nil {
		return err
	}
	d.Health = summary.BuildHealth(ctx, m.Health, repos, now)
	d.Plan = summary.BuildPlan(ctx, m.Plan, repos)
	if d.MergeErr != nil && now.Before(time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, loc).Add(giveUpAfter)) {
		m.mu.Lock()
		m.tried[t.Name] = now
		m.mu.Unlock()
		m.log().WarnContext(ctx, "morning digest waits for a complete merge list", "tenant", t.Name, "error", d.MergeErr)
		return nil
	}
	if err := m.Queue(ctx, t, d, "digest:"+t.Name+":"+day); err != nil {
		return err
	}
	m.mu.Lock()
	m.done[t.Name] = day
	m.mu.Unlock()
	return nil
}

// Queue stores the digest as a message to the owner under an idempotency key. The service
// delivers it: its loop flushes the outbox. The daily send uses one key per local day; a resend
// passes a fresh key.
func (m *Morning) Queue(ctx context.Context, t config.Tenant, d summary.Digest, key string) error {
	if t.People.Owner.Chat == "" {
		return fmt.Errorf("morning: tenant %s has no people.owner.chat", t.Name)
	}
	card, err := json.Marshal(integration.Card{
		Tenant: t.Name, Kind: integration.CardBlocker, Text: slackEscape(d.Format()),
		Thread: integration.MessageRef{Channel: t.People.Owner.Chat},
	})
	if err != nil {
		return fmt.Errorf("morning: %w", err)
	}
	return m.Outbox.Enqueue(ctx, bookdb.New(m.Pool), t.Name, 0, ChatTarget, key, card)
}

func (m *Morning) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Morning) log() *slog.Logger {
	if m.Log != nil {
		return m.Log
	}
	return slog.Default()
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
