package jobsapi_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// A refused job token on a heartbeat means the lease is gone; other refusals are not.
func TestHeartbeatReportsALostLease(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jobs/7/heartbeat" && r.Header.Get("Authorization") == "Bearer old" {
			http.Error(w, "bad or expired job token", http.StatusUnauthorized)
			return
		}
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := &jobsapi.Client{BaseURL: srv.URL, WorkerToken: "w"}
	if err := c.Heartbeat(t.Context(), "7", "old", ""); !errors.Is(err, jobsapi.ErrLeaseLost) {
		t.Fatalf("refused token: %v", err)
	}
	if err := c.Heartbeat(t.Context(), "7", "new", ""); err == nil || errors.Is(err, jobsapi.ErrLeaseLost) {
		t.Fatalf("a busy server is not a lost lease: %v", err)
	}
}
