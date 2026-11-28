package dispatch

import (
	"strings"
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

// T1, ADR 0028: the job opens the preview by its name on the preview network, which the owner's
// browser cannot resolve, so the card shows the loopback URL when the job input has one.
func TestTheCardShowsThePreviewURLThePersonCanOpen(t *testing.T) {
	task := bookdb.BookTask{ID: 7, Title: "x"}
	for payload, want := range map[string]string{
		`{"inputs":{"preview_url":"http://ns-a-7.preview.internal:80","preview_local":"http://127.0.0.1:49153"}}`: "http://127.0.0.1:49153",
		`{"inputs":{"preview_url":"http://127.0.0.1:49153"}}`:                                                     "http://127.0.0.1:49153",
	} {
		got := evidenceText(task, bookdb.BookJob{Payload: []byte(payload)}, nil, evidenceLabelsFor(false))
		if !strings.Contains(got, "Preview: "+want) || strings.Contains(got, "preview.internal") {
			t.Errorf("card for %s:\n%s", payload, got)
		}
	}
}
