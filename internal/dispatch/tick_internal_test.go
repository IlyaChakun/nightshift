package dispatch

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

func TestRunningFirst(t *testing.T) {
	t.Parallel()
	in := []bookdb.BookTask{
		{ID: 1, State: "queued"}, {ID: 2, State: "acceptance"}, {ID: 3, State: "queued"},
		{ID: 4, State: "idea"}, {ID: 5, State: "building"},
	}
	var got []int64
	for _, task := range runningFirst(in) {
		got = append(got, task.ID)
	}
	if want := []int64{2, 4, 5, 1, 3}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// A step that hangs is cut by its timeout and left for the next tick (plan R1, ADR 0018).
func TestAStepThatHangsIsCutAndLeftForTheNextTick(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	tk := &Ticker{
		Log:         slog.New(slog.NewTextHandler(&logs, nil)),
		StepTimeout: 50 * time.Millisecond,
		Step: func(ctx context.Context, _ config.Tenant, _ bookdb.BookTask) error {
			<-ctx.Done() // a call that never answers, until it is cut
			return ctx.Err()
		},
	}
	start := time.Now()
	if err := tk.step(t.Context(), config.Tenant{Name: "example-solo"}, bookdb.BookTask{ID: 7, State: "checks"}); err != nil {
		t.Fatalf("a timed-out step is not an error of the task: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("the step ran for %v", time.Since(start))
	}
	if !strings.Contains(logs.String(), "step timed out") || !strings.Contains(logs.String(), "task=7") {
		t.Errorf("the cut was not logged with the task: %q", logs.String())
	}
}

// An error of a step that is not a timeout, and a stopped service, still reach the caller.
func TestStepErrorsOtherThanTheTimeoutAreKept(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tk := &Ticker{
		Log:  slog.New(slog.DiscardHandler),
		Step: func(context.Context, config.Tenant, bookdb.BookTask) error { return boom },
	}
	if err := tk.step(t.Context(), config.Tenant{}, bookdb.BookTask{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stopped := &Ticker{
		Log:  slog.New(slog.DiscardHandler),
		Step: func(ctx context.Context, _ config.Tenant, _ bookdb.BookTask) error { return ctx.Err() },
	}
	if err := stopped.step(ctx, config.Tenant{}, bookdb.BookTask{}); !errors.Is(err, context.Canceled) {
		t.Errorf("a stopped service must see its cancellation, got %v", err)
	}
}

// The teardown sweep reads project files from fresh mirrors, so it runs once per interval per
// tenant, not on every tick.
func TestThePreviewSweepRunsOncePerInterval(t *testing.T) {
	t.Parallel()
	calls := 0
	tk := &Ticker{Log: slog.New(slog.DiscardHandler), For: func(config.Tenant) *Conductor { calls++; return &Conductor{} }}
	a, b := config.Tenant{Name: "example-solo"}, config.Tenant{Name: "example-team"}
	for range 3 {
		tk.teardownPreviews(t.Context(), a)
	}
	tk.teardownPreviews(t.Context(), b)
	if calls != 2 {
		t.Errorf("the sweep ran %d times, want once per tenant", calls)
	}
	tk.swept[a.Name] = time.Now().Add(-teardownEvery)
	tk.teardownPreviews(t.Context(), a)
	if calls != 3 {
		t.Errorf("the sweep did not run again after the interval: %d", calls)
	}
}
