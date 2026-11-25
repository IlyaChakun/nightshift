package dispatch_test

import (
	"strings"
	"testing"
	"time"

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
