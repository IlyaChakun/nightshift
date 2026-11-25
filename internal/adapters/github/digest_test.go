package github

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMergedSinceListsOnlyMergesInTheWindow(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/example/app/pulls" || r.URL.Query().Get("state") != "closed" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `[
 {"number":3,"title":"late","html_url":"https://x/3","merged_at":"2026-11-25T08:00:00Z","updated_at":"2026-11-25T08:00:00Z",
  "merged_by":{"login":"owner"},"labels":[{"name":"needs-owner-review"}],
  "body":"## What and why\n\nAdds the thing so\nthe owner can see it.\n\nMore text.\n\n## Reviewed by\nan agent"},
 {"number":2,"title":"closed unmerged","html_url":"https://x/2","merged_at":null,"updated_at":"2026-11-25T07:00:00Z","body":""},
 {"number":4,"title":"early","html_url":"https://x/4","merged_at":"2026-11-25T01:00:00Z","updated_at":"2026-11-25T02:00:00Z","body":"no template"},
 {"number":1,"title":"old","html_url":"https://x/1","merged_at":"2026-11-20T01:00:00Z","updated_at":"2026-11-20T01:00:00Z","body":""}
]`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	got, err := c.MergedSince(t.Context(), "example/app", since)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Number != 4 || got[1].Number != 3 {
		t.Fatalf("merges = %+v, want #4 then #3 (merge order, no unmerged, none before the window)", got)
	}
	m := got[1]
	if m.Reason != "Adds the thing so the owner can see it." || !m.Reviewed || m.MergedBy != "owner" || len(m.Labels) != 1 {
		t.Fatalf("merge = %+v", m)
	}
	if got[0].Reviewed || got[0].Reason != "" {
		t.Fatalf("a description without the template has no reason and no review: %+v", got[0])
	}
}

func TestMergedSinceRefusesABadRepoAndSurfacesErrors(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"nope"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	if _, err := c.MergedSince(t.Context(), "../etc", time.Now()); err == nil {
		t.Fatal("a bad repository must be refused")
	}
	if _, err := c.MergedSince(t.Context(), "example/app", time.Now()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("a refused list must be an error, not an empty digest: %v", err)
	}
}

func TestMergedSinceSaysWhenTheListingStopsAtItsCap(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var docs []string
		for i := range 100 { // every PR of every page was merged inside the window
			docs = append(docs, fmt.Sprintf(`{"number":%d,"title":"t","html_url":"u","merged_at":"2026-11-25T08:00:00Z","updated_at":"2026-11-25T09:00:00Z","body":""}`, i+1))
		}
		_, _ = fmt.Fprint(w, "["+strings.Join(docs, ",")+"]")
	}))
	defer srv.Close()
	got, err := (&Client{BaseURL: srv.URL}).MergedSince(t.Context(), "example/app", since)
	if !errors.Is(err, ErrTruncated) || len(got) != maxDigestPages*100 {
		t.Fatalf("got %d merges, err %v; want %d and ErrTruncated", len(got), err, maxDigestPages*100)
	}
}
