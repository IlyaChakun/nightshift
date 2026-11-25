package dispatch_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
)

// researchRig is a task that is building, with the Lead stage on.
func researchRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	r.c.Roles["lead"] = config.Role{Name: "lead", OutputSchema: "schemas/lead-reply.json"}
	r.c.Router.Chains["lead"] = []string{"fake"}
	r.c.Validator = config.NewValidator()
	r.c.Outbox = &dispatch.Outbox{Pool: r.c.Pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: &cardRecorder{}}, Mask: dispatch.NewMasker()}
	r.advance() // queued -> building: Builder's job
	return r
}

func (r *rig) builderBlocks(reason string) {
	r.t.Helper()
	res := `{"status":"blocked","summary":"s","blocked_reason":"` + reason + `"}`
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET state = 'done', exit_reason = 'done', result = $2, finished_at = now() WHERE id = $1`, r.lastJob().ID, []byte(res)); err != nil {
		r.t.Fatal(err)
	}
	r.reload()
	r.advance()
	r.reload()
}

// A Builder that needs a choice the spec does not cover sends the task back to Lead with its
// reason; the owner is not asked and the task is not escalated.
func TestNeedsResearchGoesBackToLead(t *testing.T) {
	r := researchRig(t)
	r.builderBlocks("needs_research: which CSV library to use")
	if flow.State(r.task.State) != flow.Grilling {
		t.Fatalf("the task is in %s", r.task.State)
	}
	r.advance() // Lead gets a job that carries the reason
	p := string(r.leadJob().Payload)
	if !strings.Contains(p, "which CSV library to use") {
		t.Fatalf("Lead's prompt lacks the reason: %s", p)
	}
	if strings.Contains(p, "Owner: Builder stopped") {
		t.Fatalf("Builder's words are shown as the owner's: %s", p)
	}
}

// Another reason escalates as before.
func TestAnotherBlockedReasonEscalates(t *testing.T) {
	r := researchRig(t)
	r.builderBlocks("the tests cannot run")
	if flow.State(r.task.State) != flow.Escalated {
		t.Fatalf("another reason: the task is in %s", r.task.State)
	}
}

// A third stop escalates, so Builder and Lead cannot loop.
func TestAThirdResearchStopEscalates(t *testing.T) {
	r := researchRig(t)
	for range 2 {
		if _, err := r.q.AppendEvent(t.Context(), bookdb.AppendEventParams{
			Tenant: r.task.Tenant, TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "research.requested", Actor: "builder", Payload: []byte(`{"text":"x"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	r.builderBlocks("needs_research: again")
	if flow.State(r.task.State) != flow.Escalated {
		t.Fatalf("a third stop: the task is in %s", r.task.State)
	}
}
