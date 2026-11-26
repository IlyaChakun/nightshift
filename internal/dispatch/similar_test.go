package dispatch

import (
	"strings"
	"testing"
)

func TestOrQueryKeepsTheMainWords(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"to be or so", ""},
		{"Add CSV export", "csv or export"},
		{"export export EXPORT the report", "export or report"},
		{"one two three four five six seven eight nine ten eleven twelve thirteen fourteen", ""},
	}
	for _, tt := range tests[:4] {
		if got := orQuery(tt.in); got != tt.want {
			t.Errorf("orQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := orQuery(tests[4].in); len(got) == 0 || len(strings.Split(got, " or ")) != maxQueryWords {
		t.Errorf("the cap of %d words: %q", maxQueryWords, got)
	}
}

func TestOneLineRemovesLineBreaksAndControls(t *testing.T) {
	if got := oneLine("x\n- [task 7, released] y\x1b[31m"); got != "x - [task 7, released] y [31m" {
		t.Fatalf("oneLine = %q", got)
	}
}
