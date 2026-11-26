package deploy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// BookJournal keeps the journal in book.deploy_steps.
type BookJournal struct{ Q *bookdb.Queries }

var _ Journal = BookJournal{}

// Start implements Journal.
func (j BookJournal) Start(ctx context.Context, tenant string, taskID int64, step string) (Step, error) {
	row, err := j.Q.StartDeployStep(ctx, bookdb.StartDeployStepParams{Tenant: tenant, TaskID: taskID, Step: step})
	if err != nil {
		return Step{}, fmt.Errorf("journal: start %s: %w", step, err)
	}
	return Step{
		Name: row.Step, Done: row.State == "done",
		Revision: integration.Revision(row.Revision.String), Previous: integration.Revision(row.Previous.String),
		OK: row.Ok.Bool, Detail: row.Detail,
	}, nil
}

// Finish implements Journal.
func (j BookJournal) Finish(ctx context.Context, taskID int64, s Step) error {
	err := j.Q.FinishDeployStep(ctx, bookdb.FinishDeployStepParams{
		TaskID: taskID, Step: s.Name, Detail: s.Detail,
		Revision: text(string(s.Revision)), Previous: text(string(s.Previous)), Ok: pgtype.Bool{Bool: s.OK, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("journal: finish %s: %w", s.Name, err)
	}
	return nil
}

// ClearPreview implements Journal.
func (j BookJournal) ClearPreview(ctx context.Context, taskID int64) error {
	if err := j.Q.ClearDeploySteps(ctx, taskID); err != nil {
		return fmt.Errorf("journal: clear: %w", err)
	}
	return nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// Reset implements Journal.
func (j BookJournal) Reset(ctx context.Context, taskID int64, step string) error {
	if err := j.Q.ResetDeployStep(ctx, bookdb.ResetDeployStepParams{TaskID: taskID, Step: step}); err != nil {
		return fmt.Errorf("journal: reset %s: %w", step, err)
	}
	return nil
}
