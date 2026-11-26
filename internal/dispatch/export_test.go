package dispatch

import (
	"context"
	"encoding/json"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
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
func SpecText(raw json.RawMessage, card bool) string { return specText(raw, card) }

// CoverageMisses lets the black-box tests call the coverage check.
func CoverageMisses(owner []string, spec json.RawMessage) []string {
	return coverageMisses(owner, spec)
}
