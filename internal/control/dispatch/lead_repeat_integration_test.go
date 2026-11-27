package dispatch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// A Lead result whose consumption fails after its events were written (here the move from
// doc_checks to spec) is consumed again on the next tick. Lead reads its own transcript, so a
// repeat must not write lead.reply, lead.docs, lead.spec or the handoff a second time.
func TestRepeatedLeadStepWritesItsEventsOnce(t *testing.T) {
	db := pgtest.Start(t)
	r, _ := leadRigOnBook(t, db)
	ctx := t.Context()
	r.advance()
	good, _ := json.Marshal([]map[string]string{{"path": "docs/prd/csv.md", "body": goodPRD}})
	withHandoff := strings.Replace(specWithDocs(string(good)), `"spec_ready"`, `"handoff":{"done":["the spec"],"pending":["the owner's answer"]},"spec_ready"`, 1)
	r.leadSays(withHandoff)

	// The second document move fails once, as on a step timeout or a restart. A sequence counts
	// the attempts because it survives the rollback of the failed one.
	for _, stmt := range []string{
		`CREATE SEQUENCE spec_moves`,
		`CREATE FUNCTION fail_spec_once() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER AS $$
		BEGIN
			IF NEW.state = 'spec' THEN
				IF nextval('spec_moves') = 1 THEN
					RAISE EXCEPTION 'injected: move to spec fails';
				END IF;
			END IF;
			RETURN NEW;
		END $$`,
		`CREATE TRIGGER fail_spec_once BEFORE UPDATE ON book.tasks FOR EACH ROW EXECUTE FUNCTION fail_spec_once()`,
	} {
		if _, err := db.Owner.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.c.Advance(ctx, r.tenant, r.task); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("the first consumption returned %v, want the injected failure", err)
	}
	r.reload()
	if r.task.State != "doc_checks" {
		t.Fatalf("after the failure the task is in %s, want doc_checks", r.task.State)
	}
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("after the repeat the task is in %s, want spec", r.task.State)
	}
	for _, kind := range []string{"lead.reply", "lead.docs", "lead.spec", "handoff"} {
		var n int
		if err := r.c.Pool.QueryRow(ctx, `SELECT count(*) FROM book.events WHERE task_id = $1 AND kind = $2`, r.task.ID, kind).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("got %d %s events, want 1", n, kind)
		}
	}
}
