package settings_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/settings"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// TestInvalidValuesNeverReachTheBook needs no database: validation runs first.
func TestInvalidValuesNeverReachTheBook(t *testing.T) {
	t.Parallel()
	s := &settings.Store{}
	for name, tc := range map[string]struct {
		tenant, key string
		value       any
	}{
		"unknown key":            {"example-solo", "colors", true},
		"pause is not a string":  {settings.Instance, settings.Paused, "yes"},
		"pause is for the whole": {"example-solo", settings.Paused, true},
		"review mode is a name":  {"example-solo", settings.ReviewMode, "sometimes"},
		"review is per tenant":   {settings.Instance, settings.ReviewMode, "all"},
		"parallelism below one":  {"example-solo", settings.Parallelism, 0},
		"share above one":        {"example-solo", settings.WeeklyShare, 1.5},
	} {
		if err := s.Set(t.Context(), tc.tenant, tc.key, tc.value, "owner"); !errors.Is(err, settings.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSetRecordsAnEventAndPauseStopsClaims(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	s := &settings.Store{Pool: pool}

	if err := s.Set(ctx, "example-solo", settings.ReviewMode, "blast_radius", "U1"); err != nil {
		t.Fatal(err)
	}
	var mode string
	if ok, err := s.Get(ctx, "example-solo", settings.ReviewMode, &mode); err != nil || !ok || mode != "blast_radius" {
		t.Fatalf("get: %q %v %v", mode, ok, err)
	}
	if ok, _ := s.Get(ctx, "example-solo", settings.Parallelism, &mode); ok {
		t.Fatal("an unset setting was found")
	}

	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Pause"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{Tenant: "example-solo", TaskID: task.ID, Role: "builder", Provider: "fake", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	claim := func() error {
		_, err := q.ClaimJob(ctx, bookdb.ClaimJobParams{
			WorkerID:    pgtype.Text{String: "w1", Valid: true},
			TokenSha256: pgtype.Text{String: "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90", Valid: true},
			LeaseUntil:  pgtype.Timestamptz{Time: pgtime(), Valid: true}, Now: pgtype.Timestamptz{Time: pgtime(), Valid: true},
		})
		return err
	}
	if err := s.Set(ctx, settings.Instance, settings.Paused, true, "U1"); err != nil {
		t.Fatal(err)
	}
	if paused, err := s.IsPaused(ctx); err != nil || !paused {
		t.Fatalf("paused: %v %v", paused, err)
	}
	if err := claim(); err == nil {
		t.Fatal("a paused factory handed out a job")
	}
	if err := s.Set(ctx, settings.Instance, settings.Paused, false, "U1"); err != nil {
		t.Fatal(err)
	}
	if err := claim(); err != nil {
		t.Fatalf("claim after resume: %v", err)
	}

	// Both pause changes are events with their author and the previous value.
	var kinds int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM book.events WHERE kind = 'settings.changed' AND actor = 'U1' AND tenant = ''`).Scan(&kinds); err != nil || kinds != 2 {
		t.Fatalf("events: %d %v", kinds, err)
	}
}

func pgtime() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }
