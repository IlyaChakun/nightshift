package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
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
		if got := flow.Exhausted(tt.b); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}

func TestTraceparent(t *testing.T) {
	t.Parallel()
	if got := Traceparent(t.Context()); got != "" {
		t.Fatalf("no span: %q", got)
	}
	tid, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	sid, _ := trace.SpanIDFromHex("b7ad6b7169203331")
	ctx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled}))
	if got, want := Traceparent(ctx), "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLimitBackoff(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]time.Duration{0: 15 * time.Minute, 1: 15 * time.Minute, 2: 30 * time.Minute, 3: time.Hour, 5: 4 * time.Hour, 6: 6 * time.Hour, 20: 6 * time.Hour} {
		if got := LimitBackoff(n); got != want {
			t.Errorf("LimitBackoff(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestRouterFallback(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	near, far := now.Add(time.Hour), now.Add(5*time.Hour)
	tests := []struct {
		name    string
		role    string
		allowed bool
		limits  map[string]time.Time
		want    Choice
	}{
		{name: "primary free: no fallback", role: "builder", allowed: true, want: Choice{Decision: Run, Provider: "claude-sub"}},
		{name: "near reset: wait, do not fall back", role: "builder", allowed: true,
			limits: map[string]time.Time{"claude-sub": near}, want: Choice{Decision: Park, Until: near}},
		{name: "far reset: fall back when allowed", role: "builder", allowed: true,
			limits: map[string]time.Time{"claude-sub": far}, want: Choice{Decision: Run, Provider: "deepseek"}},
		{name: "far reset: wait when the task did not opt in", role: "builder", allowed: false,
			limits: map[string]time.Time{"claude-sub": far}, want: Choice{Decision: Park, Until: far}},
		{name: "Lead never falls back", role: "lead", allowed: true,
			limits: map[string]time.Time{"claude-sub": far}, want: Choice{Decision: Park, Until: far}},
		{name: "fallback also limited: park until the earliest reset", role: "builder", allowed: true,
			limits: map[string]time.Time{"claude-sub": far, "deepseek": near}, want: Choice{Decision: Park, Until: near}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := testRouter(nil, now)
			r.Providers["deepseek"] = config.Provider{Harness: "codex", Auth: "api_key", Tier: "fallback"}
			r.Chains["builder"] = []string{"claude-sub", "deepseek"}
			r.Chains["lead"] = []string{"claude-sub", "deepseek"}
			for p, until := range tt.limits {
				r.NoteLimit(p, until)
			}
			got, err := r.ChooseFor(t.Context(), tt.role, tt.allowed)
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestRouterChainOfOnlyFallbackProviders(t *testing.T) {
	t.Parallel()
	r := testRouter(nil, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	r.Providers["deepseek"] = config.Provider{Harness: "codex", Auth: "api_key", Tier: "fallback"}
	r.Chains["builder"] = []string{"deepseek"}
	if _, err := r.ChooseFor(t.Context(), "builder", false); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("not opted in: want ErrNoProvider, got %v", err)
	}
	if c, err := r.ChooseFor(t.Context(), "builder", true); err != nil || c.Provider != "deepseek" {
		t.Fatalf("opted in: got %+v, %v", c, err)
	}
}

func TestRouterWaitsForEligiblePrimaryBeforeEnqueue(t *testing.T) {
	capUSD := 10.0
	busy := true
	r := Router{
		Providers:   map[string]config.Provider{"metered": {Pool: "credits"}, "other": {Concurrency: 1}, "fallback": {Tier: "fallback"}},
		CreditPools: config.CreditPools{"credits": {MonthlyUSD: &capUSD}},
		Chains:      map[string][]string{"builder": {"metered", "other", "fallback"}},
		InFlight: func(context.Context, string) (int, error) {
			if busy {
				return 1, nil
			}
			return 0, nil
		},
	}
	if c, err := r.ChooseFor(t.Context(), "builder", false); err != nil || c.Decision != Wait {
		t.Fatalf("busy: %+v, %v", c, err)
	}
	busy = false
	if c, err := r.ChooseFor(t.Context(), "builder", false); err != nil || c.Decision != Run || c.Provider != "other" {
		t.Fatalf("free: %+v, %v", c, err)
	}
}
