package fake_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/tracker/fake"
	"github.com/ilyachakun/nightshift/internal/control/trackerapi"
)

func TestUpsertKeepsOneIssuePerTask(t *testing.T) {
	t.Parallel()
	tr := fake.New(nil)
	a, _ := tr.Upsert(t.Context(), trackerapi.TaskView{Tenant: "example-solo", TaskID: "1", Title: "A", State: "queued"})
	b, _ := tr.Upsert(t.Context(), trackerapi.TaskView{Tenant: "example-solo", TaskID: "1", Title: "A", State: "building"})
	c, _ := tr.Upsert(t.Context(), trackerapi.TaskView{Tenant: "example-solo", TaskID: "2", Title: "B"})
	if a.ID != b.ID || a.ID == c.ID {
		t.Fatalf("refs: %+v %+v %+v", a, b, c)
	}
	if v, _ := tr.Issue(a.ID); v.State != "building" {
		t.Fatalf("issue: %+v", v)
	}
}

func TestPollReturnsOnlyNewChangesInOrder(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	tr := fake.New(func() time.Time { clock = clock.Add(time.Minute); return clock })
	ref, _ := tr.Upsert(t.Context(), trackerapi.TaskView{Tenant: "example-solo", TaskID: "1"})
	start := clock
	if err := tr.SetPriority(ref.ID, "owner", "P0"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Cancel(ref.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	got, err := tr.Poll(t.Context(), start)
	if err != nil || len(got) != 2 || got[0].Kind != trackerapi.TrackerPriorityChanged || got[0].Priority != "P0" || got[1].Kind != trackerapi.TrackerCanceled {
		t.Fatalf("poll: %+v %v", got, err)
	}
	if later, _ := tr.Poll(t.Context(), got[1].At); len(later) != 0 {
		t.Fatalf("poll after the last change: %+v", later)
	}
	if err := tr.Cancel("FAKE-99", "owner"); !errors.Is(err, fake.ErrUnknown) {
		t.Fatalf("unknown issue: %v", err)
	}
}
