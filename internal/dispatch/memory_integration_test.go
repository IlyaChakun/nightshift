package dispatch_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/memory"
)

// A fact the owner gave once is in the prompt of Lead and of Builder, so it is not asked again;
// an expired entry is not.
func TestProjectMemoryGoesIntoLeadAndBuilderPrompts(t *testing.T) {
	r, _ := leadRig(t)
	ctx := t.Context()
	now := time.Now()
	err := memory.Store{Pool: r.c.Pool}.Write(ctx, r.task.Tenant, 0, []memory.Entry{
		{Key: "tests", Text: "tests run with make test on this machine", Source: "owner:U1", ExpiresAt: now.Add(time.Hour)},
		{Key: "old", Text: "the staging host is blue", Source: "owner:U1", ExpiresAt: now.Add(-time.Hour)},
		{Key: "pinned", Text: "no new dependencies without asking", Source: "repo:AGENTS.md"},
	}, "U1", "test")
	if err != nil {
		t.Fatal(err)
	}
	r.advance() // idea -> grilling: Lead's job
	p := string(r.leadJob().Payload)
	for _, want := range []string{"Project memory", "tests run with make test on this machine", "no new dependencies without asking"} {
		if !strings.Contains(p, want) {
			t.Errorf("Lead's prompt lacks %q: %s", want, p)
		}
	}
	if strings.Contains(p, "the staging host is blue") {
		t.Errorf("an expired entry reached Lead's prompt: %s", p)
	}
}

func TestProjectMemoryGoesIntoBuilderPrompt(t *testing.T) {
	r := newRig(t)
	err := memory.Store{Pool: r.c.Pool}.Write(t.Context(), r.task.Tenant, 0, []memory.Entry{
		{Key: "tests", Text: "tests run with make test on this machine", Source: "owner:U1", ExpiresAt: time.Now().Add(time.Hour)},
	}, "U1", "test")
	if err != nil {
		t.Fatal(err)
	}
	r.advance() // queued -> building: Builder's job
	if p := string(r.lastJob().Payload); !strings.Contains(p, "tests run with make test on this machine") {
		t.Fatalf("Builder's prompt lacks the page: %s", p)
	}
}

// The prune runs once a day per tenant from the ticker: an expired entry goes on the first tick,
// and a second one written the same day waits for the next day.
func TestTickPrunesTheMemoryPageOncePerDay(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	store := memory.Store{Pool: r.c.Pool}
	expired := func(key string) memory.Entry {
		return memory.Entry{Key: key, Text: "fact " + key, Source: "owner:U1", ExpiresAt: time.Now().Add(-time.Hour)}
	}
	if err := store.Write(ctx, r.task.Tenant, 0, []memory.Entry{expired("a")}, "U1", "seed"); err != nil {
		t.Fatal(err)
	}
	tk := &dispatch.Ticker{
		Pool: r.c.Pool, Tenants: map[string]config.Tenant{r.task.Tenant: r.tenant},
		For: func(config.Tenant) *dispatch.Conductor { return r.c }, Log: slog.Default(),
	}
	tk.Tick(ctx)
	if p, _ := store.Read(ctx, r.task.Tenant); len(p.Entries) != 0 {
		t.Fatalf("the expired entry stayed: %+v", p.Entries)
	}
	p, _ := store.Read(ctx, r.task.Tenant)
	if err := store.Write(ctx, r.task.Tenant, p.Revision, []memory.Entry{expired("b")}, "U1", "seed"); err != nil {
		t.Fatal(err)
	}
	tk.Tick(ctx)
	if p, _ = store.Read(ctx, r.task.Tenant); len(p.Entries) != 1 {
		t.Fatalf("a second prune on the same day: %+v", p.Entries)
	}
}
