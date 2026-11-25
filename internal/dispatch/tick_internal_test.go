package dispatch

import (
	"slices"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

func TestRunningFirst(t *testing.T) {
	t.Parallel()
	in := []bookdb.BookTask{
		{ID: 1, State: "queued"}, {ID: 2, State: "acceptance"}, {ID: 3, State: "queued"},
		{ID: 4, State: "idea"}, {ID: 5, State: "building"},
	}
	var got []int64
	for _, task := range runningFirst(in) {
		got = append(got, task.ID)
	}
	if want := []int64{2, 4, 5, 1, 3}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}
