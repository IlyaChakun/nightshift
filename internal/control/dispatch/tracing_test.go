package dispatch

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/routing"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

func TestTaskContextIsStablePerTask(t *testing.T) {
	t.Parallel()
	at := pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}
	task := func(id int64) bookdb.BookTask {
		return bookdb.BookTask{ID: id, Tenant: "example-solo", CreatedAt: at}
	}
	first, again, other := routing.Traceparent(taskContext(t.Context(), task(1))), routing.Traceparent(taskContext(t.Context(), task(1))), routing.Traceparent(taskContext(t.Context(), task(2)))
	if first == "" || first != again || first == other {
		t.Errorf("traceparent: %q, %q, other task %q", first, again, other)
	}
}
