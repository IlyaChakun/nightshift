package jobsapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestRoutesAreValidMuxPatterns registers every route on a ServeMux; a
// malformed or duplicate pattern panics.
func TestRoutesAreValidMuxPatterns(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	for _, r := range []string{RouteClaim, RouteHeartbeat, RouteEvents, RouteComplete, RouteSearch, RouteStandards, RouteTask} {
		mux.HandleFunc(r, func(http.ResponseWriter, *http.Request) {})
	}
}

// TestVisibleInputs is the T6 rule that a Builder never sees hidden scenarios.
func TestVisibleInputs(t *testing.T) {
	t.Parallel()
	in := json.RawMessage(`{"spec":"s","hidden_scenarios":["x"]}`)
	tests := []struct {
		name, role string
		in         json.RawMessage
		want       string
	}{
		{"builder", "builder", in, `{"spec":"s"}`},
		{"lead", "lead", in, `{"spec":"s"}`},
		{"verifier", "verifier", in, `{"hidden_scenarios":["x"],"spec":"s"}`},
		{"no inputs", "builder", nil, ``},
		{"unreadable fails closed", "builder", json.RawMessage(`[1`), ``},
	}
	for _, tt := range tests {
		if got := string(VisibleInputs(tt.role, tt.in)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
