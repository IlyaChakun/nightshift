package jobsapi

import (
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
