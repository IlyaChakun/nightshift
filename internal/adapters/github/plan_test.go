package github

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlanProgressCountsChecklistItemsPerFile(t *testing.T) {
	t.Parallel()
	text := "# Phase 5 — Lead\n\n- [x] A1. done\n- [x] A2. done\n- [ ] Q1. open\nprose - [ ] not an item\n"
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/app/contents/docs/plan":
			_, _ = fmt.Fprint(w, `[{"name":"phase-5.md","path":"docs/plan/phase-5.md","type":"file"},{"name":"notes.txt","path":"docs/plan/notes.txt","type":"file"}]`)
		case "/repos/example/app/contents/docs/plan/phase-5.md":
			_, _ = fmt.Fprintf(w, `{"encoding":"base64","content":%q}`, enc[:10]+"\n"+enc[10:])
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	got, err := c.PlanProgress(t.Context(), "example/app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Phase 5" || got[0].Done != 2 || got[0].Total != 3 {
		t.Fatalf("got %+v, want Phase 5 with 2 of 3", got)
	}
}

func TestPlanProgressOfARepositoryWithoutAPlanIsEmpty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	got, err := (&Client{BaseURL: srv.URL}).PlanProgress(t.Context(), "example/app")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v; want nothing and no error", got, err)
	}
}
