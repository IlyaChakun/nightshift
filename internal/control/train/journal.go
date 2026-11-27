package train

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// BookJournal keeps the journal in book.train_steps.
type BookJournal struct{ Q *bookdb.Queries }

// Start implements Journal.
func (j BookJournal) Start(ctx context.Context, tenant string, taskID int64, step string) (Step, error) {
	rows, err := j.Q.ListTrainSteps(ctx, taskID)
	if err != nil {
		return Step{}, fmt.Errorf("journal: list: %w", err)
	}
	for _, r := range rows {
		if r.Step == step {
			return Step{Name: r.Step, Done: r.State == "done", SHA: r.Sha.String}, nil
		}
	}
	row, err := j.Q.StartTrainStep(ctx, bookdb.StartTrainStepParams{Tenant: tenant, TaskID: taskID, Step: step})
	if err != nil {
		return Step{}, fmt.Errorf("journal: start %s: %w", step, err)
	}
	return Step{Name: row.Step, Fresh: true}, nil
}

// Peek implements Journal.
func (j BookJournal) Peek(ctx context.Context, taskID int64, step string) (Step, bool, error) {
	rows, err := j.Q.ListTrainSteps(ctx, taskID)
	if err != nil {
		return Step{}, false, fmt.Errorf("journal: list: %w", err)
	}
	for _, r := range rows {
		if r.Step == step {
			return Step{Name: r.Step, Done: r.State == "done", SHA: r.Sha.String}, true, nil
		}
	}
	return Step{}, false, nil
}

// Finish implements Journal.
func (j BookJournal) Finish(ctx context.Context, taskID int64, step, sha string) error {
	if err := j.Q.FinishTrainStep(ctx, bookdb.FinishTrainStepParams{TaskID: taskID, Step: step, Sha: pgtype.Text{String: sha, Valid: true}}); err != nil {
		return fmt.Errorf("journal: finish %s: %w", step, err)
	}
	return nil
}

// Clear implements Journal.
func (j BookJournal) Clear(ctx context.Context, taskID int64) error {
	if err := j.Q.ClearTrainSteps(ctx, taskID); err != nil {
		return fmt.Errorf("journal: clear: %w", err)
	}
	return nil
}
