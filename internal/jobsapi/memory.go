package jobsapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
)

// Limits on what one result may add to the book, so a runaway agent cannot flood it.
const (
	maxNotes    = 5
	maxFindings = 200
)

// memory is the part of a role result the book keeps: findings and notes. Each element is
// validated on its own, so one bad element does not lose the others or fail the job.
type memory struct {
	Findings []json.RawMessage `json:"findings"`
	Notes    []json.RawMessage `json:"notes"`
}

type findingDoc struct {
	RuleID        string `json:"rule_id"`
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	LayerCaught   string `json:"layer_caught"`
	LayerExpected string `json:"layer_expected"`
	Message       string `json:"message"`
	Location      *struct {
		Path      string `json:"path"`
		StartLine int32  `json:"start_line"`
		EndLine   int32  `json:"end_line"`
	} `json:"location"`
	Fingerprint string `json:"fingerprint"`
}

type noteDoc struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Source string `json:"source"`
}

// storeMemory records the findings of a role result and its notes as proposed, with the
// provenance of the job. The agent never writes to the book: the worker is not trusted, so
// every element is checked against schemas/ here. It returns how many elements it dropped.
func storeMemory(ctx context.Context, q *bookdb.Queries, v *config.Validator, job bookdb.BookJob, result json.RawMessage, checks []json.RawMessage) (dropped int, err error) {
	var m memory
	if len(result) > 0 {
		// The result schema is the runner's check; a result that holds no findings or notes
		// has nothing to store, and that is not a reason to fail the job.
		if json.Unmarshal(result, &m) != nil {
			m = memory{}
		}
	}
	m.Findings = append(m.Findings, checks...)
	for i, raw := range m.Findings {
		if i >= maxFindings || v.ValidateJSON(config.KindFinding, raw) != nil {
			dropped++
			continue
		}
		var f findingDoc
		if err := json.Unmarshal(raw, &f); err != nil {
			dropped++
			continue
		}
		p := bookdb.AddFindingParams{
			Tenant: job.Tenant, TaskID: job.TaskID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
			RuleID: f.RuleID, Category: f.Category, Severity: f.Severity, LayerCaught: f.LayerCaught,
			LayerExpected: pgtype.Text{String: f.LayerExpected, Valid: f.LayerExpected != ""},
			Message:       f.Message, Fingerprint: f.Fingerprint,
		}
		if f.Location != nil {
			p.Path = pgtype.Text{String: f.Location.Path, Valid: true}
			p.StartLine = pgtype.Int4{Int32: f.Location.StartLine, Valid: f.Location.StartLine > 0}
			p.EndLine = pgtype.Int4{Int32: f.Location.EndLine, Valid: f.Location.EndLine > 0}
		}
		if _, err := q.AddFinding(ctx, p); err != nil {
			return dropped, fmt.Errorf("jobsapi: store finding: %w", err)
		}
	}
	for i, raw := range m.Notes {
		var n noteDoc
		if i >= maxNotes || v.ValidateJSON(config.KindNote, raw) != nil || json.Unmarshal(raw, &n) != nil {
			dropped++
			continue
		}
		if _, err := q.AddNote(ctx, bookdb.AddNoteParams{
			Tenant: job.Tenant, TaskID: job.TaskID, JobID: job.ID, Role: job.Role,
			Kind: n.Kind, Body: n.Text, Source: n.Source,
		}); err != nil {
			return dropped, fmt.Errorf("jobsapi: store note: %w", err)
		}
	}
	return dropped, nil
}
