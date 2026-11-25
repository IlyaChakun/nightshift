package dispatch_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestAddTaskQueuesWithItsEvent(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	tenant := soloTenant()

	task, err := dispatch.AddTask(t.Context(), pool, tenant, "", "  Add greet.sh\nand more ", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "queued" || task.Repo != "example-owner/example-solo" || task.Title != "Add greet.sh" {
		t.Errorf("task: %+v", task)
	}
	events, err := bookdb.New(pool).ListTaskEvents(t.Context(), pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil || len(events) != 1 || events[0].Kind != "task_created" || events[0].Actor != "cli" {
		t.Errorf("events: %+v, %v", events, err)
	}
	for _, bad := range [][2]string{{"other/repo", "x"}, {"", "  "}} {
		if _, err := dispatch.AddTask(t.Context(), pool, tenant, bad[0], bad[1], "cli"); !errors.Is(err, dispatch.ErrNewTask) {
			t.Errorf("%q: want a refusal, got %v", bad, err)
		}
	}
}
