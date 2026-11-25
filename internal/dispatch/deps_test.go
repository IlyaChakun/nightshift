package dispatch

import (
	"strings"
	"testing"
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
