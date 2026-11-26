package dispatch_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestReturnedVerifierFindingHasOneRow(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance()

	ctx := t.Context()
	job := r.verifierJob()
	result := `{"verdict":"return","summary":"unexpected row count","findings":[{
		"rule_id":"csv.rows","category":"code","severity":"block","layer_caught":"L5","layer_expected":"L2",
		"message":"the export drops the last row","location":{"path":"feature.txt","start_line":1.0,"end_line":3e0},
		"fingerprint":"csv:missing-last-row"}]}`
	if err := config.NewValidator().ValidateJSON(config.KindVerdict, []byte(result)); err != nil {
		t.Fatal(err)
	}
	r.verdict(result)

	check := func(stage string) int64 {
		t.Helper()
		rows, err := r.q.ListJobFindings(ctx, pgtype.Int8{Int64: job.ID, Valid: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("%s: got %d finding rows, want one", stage, len(rows))
		}
		f := rows[0]
		if f.TaskID != r.task.ID || f.Tenant != r.task.Tenant || !f.JobID.Valid || f.JobID.Int64 != job.ID ||
			f.RuleID != "csv.rows" || f.Category != "code" || f.Severity != "block" ||
			f.LayerCaught != "L5" || !f.LayerExpected.Valid || f.LayerExpected.String != "L2" ||
			f.Message != "the export drops the last row" || f.Fingerprint != "csv:missing-last-row" ||
			!f.Path.Valid || f.Path.String != "feature.txt" ||
			!f.StartLine.Valid || f.StartLine.Int32 != 1 || !f.EndLine.Valid || f.EndLine.Int32 != 3 {
			t.Fatalf("%s: finding fields changed: %+v", stage, f)
		}
		return f.ID
	}
	id := check("after completion")
	r.advance()
	if r.task.State != "building" {
		t.Fatalf("returned verdict left task in %s", r.task.State)
	}
	if got := check("after dispatch"); got != id {
		t.Fatalf("dispatch replaced finding %d with %d", id, got)
	}
}
