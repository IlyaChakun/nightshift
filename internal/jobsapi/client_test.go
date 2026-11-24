package jobsapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// TestClientTalksToTheServerRoutes runs the client against a stub with the real routes.
func TestClientTalksToTheServerRoutes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc(jobsapi.RouteClaim, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer worker" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(jobsapi.ClaimResponse{JobID: "5", JobToken: "jt"})
	})
	mux.HandleFunc(jobsapi.RouteComplete, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer jt" || r.PathValue("id") != "5" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &jobsapi.Client{BaseURL: srv.URL, WorkerToken: "worker"}
	got, err := c.Claim(t.Context(), "w1", time.Second)
	if err != nil || got.JobID != "5" || got.JobToken != "jt" {
		t.Fatalf("claim: %+v %v", got, err)
	}
	if err := c.Complete(t.Context(), got.JobID, got.JobToken, jobsapi.CompleteRequest{ExitReason: "done"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Complete(t.Context(), got.JobID, "wrong", jobsapi.CompleteRequest{ExitReason: "done"}); err == nil {
		t.Fatal("a wrong token was accepted")
	}
	bad := &jobsapi.Client{BaseURL: srv.URL, WorkerToken: "nope"}
	if _, err := bad.Claim(t.Context(), "w1", 0); err == nil {
		t.Fatal("a wrong worker token was accepted")
	}
}
