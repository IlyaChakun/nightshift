package dispatch_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// failNextConsume makes the next consumption of a job fail once, as on a restart between the
// events and the consumption. A sequence counts the attempts because it survives the rollback.
func failNextConsume(t *testing.T, db *pgtest.DB) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE SEQUENCE consume_attempts`,
		`CREATE FUNCTION fail_consume_once() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN
			IF OLD.consumed_at IS NULL AND NEW.consumed_at IS NOT NULL THEN
				IF nextval('consume_attempts') = 1 THEN
					RAISE EXCEPTION 'injected: consume fails';
				END IF;
			END IF;
			RETURN NEW;
		END $$`,
		`CREATE TRIGGER fail_consume_once BEFORE UPDATE ON book.jobs FOR EACH ROW EXECUTE FUNCTION fail_consume_once()`,
	} {
		if _, err := db.Owner.ExecContext(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
}

// failingAdvance runs one step that must hit the injected failure.
func (r *rig) failingAdvance() {
	r.t.Helper()
	if err := r.c.Advance(r.t.Context(), r.tenant, r.task); err == nil || !strings.Contains(err.Error(), "injected") {
		r.t.Fatalf("got %v, want the injected failure", err)
	}
	r.reload()
}

func (r *rig) countEvents(kind string) int {
	r.t.Helper()
	var n int
	if err := r.c.Pool.QueryRow(r.t.Context(), `SELECT count(*) FROM book.events WHERE task_id = $1 AND kind = $2`, r.task.ID, kind).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

// A broken Lead reply goes back once. When its consumption is repeated, the repeat must not
// count its own rejection as the earlier one and tell the owner instead.
func TestRepeatedRejectionStillGivesLeadItsCorrection(t *testing.T) {
	db := pgtest.Start(t)
	r, _ := leadRigOnBook(t, db)
	r.advance()
	r.leadSays(`{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one."}`)
	failNextConsume(t, db)
	r.failingAdvance()
	r.advance() // the repeat sends it back
	if got := r.countEvents("lead.rejected"); got != 1 {
		t.Errorf("got %d lead.rejected events, want 1", got)
	}
	if got := r.countEvents("lead.failed"); got != 0 {
		t.Errorf("got %d lead.failed events, want 0: the repeat used up the correction", got)
	}
}

// A second broken reply tells the owner; a repeat of that consumption writes lead.failed once.
func TestRepeatedLeadFailureIsRecordedOnce(t *testing.T) {
	db := pgtest.Start(t)
	r, _ := leadRigOnBook(t, db)
	r.advance()
	r.leadSays(`{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one."}`)
	r.advance() // sends it back
	r.advance() // queues the correction
	r.leadSays(`{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one again."}`)
	failNextConsume(t, db)
	r.failingAdvance()
	r.advance()
	if got := r.countEvents("lead.failed"); got != 1 {
		t.Errorf("got %d lead.failed events, want 1", got)
	}
}

// Builder's research stop is counted toward maxResearchReturns once, even when its consumption
// is repeated.
func TestRepeatedResearchStopCountsOnce(t *testing.T) {
	db := pgtest.Start(t)
	r := researchRigOnBook(t, db, true)
	res := `{"status":"blocked","summary":"s","blocked_reason":"needs_research: which CSV library to use"}`
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'done', exit_reason = 'done', result = $2, finished_at = now() WHERE id = $1`, r.lastJob().ID, []byte(res)); err != nil {
		t.Fatal(err)
	}
	failNextConsume(t, db)
	r.failingAdvance()
	r.advance()
	if r.task.State != "grilling" {
		t.Fatalf("the task is in %s, want grilling", r.task.State)
	}
	if got := r.countEvents("research.requested"); got != 1 {
		t.Errorf("got %d research.requested events, want 1", got)
	}
}
