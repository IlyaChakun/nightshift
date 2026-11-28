package dispatch_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// A Builder job that ended without a result, as when its worker is stopped mid-task, escalates
// as job_failed: the owner must not read it as a spent budget (V3 on the Mac, 2026-11-28).
func TestAStoppedJobEscalatesAsAFailedJob(t *testing.T) {
	r := newRig(t)
	r.advance() // queues the Builder job
	job := r.lastJob()
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'failed', exit_reason = 'error', finished_at = now() WHERE id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	r.advance()
	if r.task.State != "escalated" {
		t.Fatalf("the task is in %s, want escalated", r.task.State)
	}
	events, err := r.q.ListTaskEvents(t.Context(), pgtype.Int8{Int64: r.task.ID, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	last := ""
	for _, e := range events {
		if e.Kind == "task.moved" {
			last = string(e.Payload)
		}
	}
	if !strings.Contains(last, `"job_failed"`) {
		t.Errorf("the escalation was recorded as %s, want job_failed", last)
	}
	if !r.lastJob().ConsumedAt.Valid {
		t.Error("the failed job was not consumed")
	}
}
