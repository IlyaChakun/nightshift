package dispatch

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

func TestCheckTaskGraph(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, spec, wantErr string
	}{
		{"one task", `{"tasks":[{"id":"T-1","title":"a"}]}`, ""},
		{"chain", `{"tasks":[{"id":"T-1","title":"a"},{"id":"T-2","title":"b","depends_on":["T-1"]},{"id":"T-3","title":"c","depends_on":["T-1","T-2"]}]}`, ""},
		{"duplicate id", `{"tasks":[{"id":"T-1","title":"a"},{"id":"T-1","title":"b"}]}`, "repeats"},
		{"unknown", `{"tasks":[{"id":"T-1","title":"a","depends_on":["T-9"]}]}`, "does not have"},
		{"self", `{"tasks":[{"id":"T-1","title":"a","depends_on":["T-1"]}]}`, "itself"},
		{"cycle", `{"tasks":[{"id":"T-1","title":"a","depends_on":["T-2"]},{"id":"T-2","title":"b","depends_on":["T-1"]}]}`, "cycle"},
		{"not json", `nope`, "not JSON"},
	} {
		err := checkTaskGraph([]byte(tc.spec))
		if (err == nil) != (tc.wantErr == "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: err %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestCheckTaskRepos(t *testing.T) {
	t.Parallel()
	tenant := config.Tenant{Repos: []string{"github.com/org/app"}}
	if err := checkTaskRepos(tenant, []byte(`{"tasks":[{"id":"T-1","title":"a","repo":"github.com/org/app"},{"id":"T-2","title":"b"}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := checkTaskRepos(tenant, []byte(`{"tasks":[{"id":"T-1","title":"a","repo":"github.com/org/other"}]}`)); err == nil {
		t.Fatal("a repository outside the tenant must be refused")
	}
}

func TestGoingOnWithoutACancelledDependencyDropsOnlyThatLink(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	q := bookdb.New(pool)
	mk := func(state string) int64 {
		var id int64
		if err := pool.QueryRow(t.Context(), `INSERT INTO book.tasks (tenant, repo, title, state) VALUES ('t','o/r','x',$1) RETURNING id`, state).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	task, gone, live := mk("blocked"), mk("canceled"), mk("building")
	for _, d := range []int64{gone, live} {
		if err := q.AddTaskDep(t.Context(), bookdb.AddTaskDepParams{TaskID: task, DependsOn: d}); err != nil {
			t.Fatal(err)
		}
	}
	if err := DropBrokenDeps(t.Context(), q, task); err != nil {
		t.Fatal(err)
	}
	rows, err := q.TaskDepStates(t.Context(), task)
	if err != nil || len(rows) != 1 || rows[0].ID != live {
		t.Fatalf("left %v, err %v", rows, err)
	}
}
