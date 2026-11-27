package dispatch_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/doors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

func TestAddTaskQueuesWithItsEvent(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	tenant := soloTenant()

	task, err := doors.AddTask(t.Context(), pool, tenant, "", "  Add greet.sh\nand more ", "cli", doors.Plan{})
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
		if _, err := doors.AddTask(t.Context(), pool, tenant, bad[0], bad[1], "cli", doors.Plan{}); !errors.Is(err, doors.ErrNewTask) {
			t.Errorf("%q: want a refusal, got %v", bad, err)
		}
	}
}

func TestAddTaskPlan(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	tenant := soloTenant()
	at := time.Date(2026, 11, 26, 22, 0, 0, 0, time.UTC)
	task, err := doors.AddTask(t.Context(), pool, tenant, "", "Plan me", "cli", doors.Plan{NotBefore: at, Priority: 5})
	if err != nil || !task.NotBefore.Valid || !task.NotBefore.Time.Equal(at) || task.Priority != 5 {
		t.Fatalf("task %+v, %v", task, err)
	}
	if _, err := doors.AddTask(t.Context(), pool, tenant, "", "x", "cli", doors.Plan{Priority: 11}); !errors.Is(err, doors.ErrNewTask) {
		t.Errorf("priority 11: want a refusal, got %v", err)
	}
}
