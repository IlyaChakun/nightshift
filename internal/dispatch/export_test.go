package dispatch

import (
	"context"

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
