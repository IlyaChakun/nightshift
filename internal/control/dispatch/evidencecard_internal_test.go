package dispatch

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// T1, review finding on P5b: a harness without a price reports cost 0, which is not a free job.
func TestJobMetricsLeaveOutAnUnknownCost(t *testing.T) {
	start := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	job := bookdb.BookJob{
		StartedAt: pgtype.Timestamptz{Time: start, Valid: true}, FinishedAt: pgtype.Timestamptz{Time: start.Add(90 * time.Second), Valid: true},
		Usage: []byte(`{"input_tokens":10,"cost_usd":0}`),
	}
	if got := jobMetrics(job); got != "1m30s" {
		t.Fatalf("metrics %q", got)
	}
	job.Usage = []byte(`{"cost_usd":1.5}`)
	if got := jobMetrics(job); got != "1m30s, $1.50" {
		t.Fatalf("metrics %q", got)
	}
}
