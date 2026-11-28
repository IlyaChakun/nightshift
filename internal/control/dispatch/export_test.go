package dispatch

import (
	"context"
	"encoding/json"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// StartFrom lets the black-box tests ask where a Builder round would start.
func (c *Conductor) StartFrom(ctx context.Context, task bookdb.BookTask, base string) (string, error) {
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return "", err
	}
	return c.startFrom(ctx, mirror, task, base), nil
}

// SpecText renders a spec for the black-box tests.
func SpecText(raw json.RawMessage, card bool) string { return specText(raw, card, labelsFor(false)) }

// CoverageMisses lets the black-box tests call the coverage check.
func CoverageMisses(owner []string, spec json.RawMessage) []string {
	return coverageMisses(owner, spec)
}

// SpecButtons returns the buttons of a spec card for the black-box tests.
func SpecButtons(spec json.RawMessage) []chatapi.Button { return specButtons(spec) }

// TrainFrozen lets the black-box tests ask whether the train of a task's repository holds.
func TrainFrozen(ctx context.Context, c *Conductor, task bookdb.BookTask) (bool, error) {
	return c.trainFrozen(ctx, task)
}

// TellRolledBack lets the black-box tests queue the rollback card.
func (c *Conductor) TellRolledBack(ctx context.Context, task bookdb.BookTask, bad, revertURL string) {
	c.tellRolledBack(ctx, task, bad, revertURL)
}

// RolledBackText renders the rollback card for the tests.
func RolledBackText(ru bool, id int64, bad, prev, why, url string) string {
	return rolledBackText(ru, id, bad, prev, why, url)
}
