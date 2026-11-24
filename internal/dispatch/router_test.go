package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
)

func testRouter(inFlight map[string]int, now time.Time) *Router {
	return &Router{
		Providers: map[string]config.Provider{
			"claude-sub": {Harness: "claude", Auth: "subscription", Concurrency: 1},
			"claude-api": {Harness: "claude", Auth: "api_key"},
		},
		Chains:   map[string][]string{"builder": {"claude-sub", "claude-api"}, "solo": {"claude-sub"}},
		InFlight: func(_ context.Context, p string) (int, error) { return inFlight[p], nil },
		Now:      func() time.Time { return now },
	}
}

func TestRouterChoose(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	tests := []struct {
		name     string
		role     string
		inFlight map[string]int
		limits   map[string]time.Time
		want     Choice
	}{
		{name: "first provider", role: "builder", want: Choice{Decision: Run, Provider: "claude-sub"}},
		{name: "subscription at its cap, API takes over", role: "builder", inFlight: map[string]int{"claude-sub": 1}, want: Choice{Decision: Run, Provider: "claude-api"}},
		{name: "limit switches to the next provider", role: "builder", limits: map[string]time.Time{"claude-sub": reset}, want: Choice{Decision: Run, Provider: "claude-api"}},
		{name: "a limit in the past is forgotten", role: "builder", limits: map[string]time.Time{"claude-sub": now.Add(-time.Minute)}, want: Choice{Decision: Run, Provider: "claude-sub"}},
		{name: "only provider busy: wait", role: "solo", inFlight: map[string]int{"claude-sub": 1}, want: Choice{Decision: Wait}},
		{name: "all limited: park until the earliest reset", role: "builder",
			limits: map[string]time.Time{"claude-sub": reset, "claude-api": now.Add(time.Hour)}, want: Choice{Decision: Park, Until: now.Add(time.Hour)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := testRouter(tt.inFlight, now)
			for p, until := range tt.limits {
				r.NoteLimit(p, until)
			}
			got, err := r.Choose(t.Context(), tt.role)
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestRouterErrors(t *testing.T) {
	t.Parallel()
	r := testRouter(nil, time.Now())
	if _, err := r.Choose(t.Context(), "lead"); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("a role without providers: %v", err)
	}
	r.Chains["lead"] = []string{"ghost"}
	if _, err := r.Choose(t.Context(), "lead"); err == nil {
		t.Fatal("an unknown provider was accepted")
	}
}

func TestExhausted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		b    bookdb.BookBudget
		want bool
	}{
		{"within", bookdb.BookBudget{MaxIterations: 5, UsedIterations: 4, MaxTokens: 100, UsedTokens: 99}, false},
		{"iterations used", bookdb.BookBudget{MaxIterations: 5, UsedIterations: 5}, true},
		{"tokens used", bookdb.BookBudget{MaxTokens: 100, UsedTokens: 150}, true},
		{"no limits", bookdb.BookBudget{UsedIterations: 99, UsedTokens: 1 << 40}, false},
	}
	for _, tt := range tests {
		if got := exhausted(tt.b); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}
