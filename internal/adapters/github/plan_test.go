package github

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestAPlanFileWithoutAPhaseHeadingIsNamedByItsFile(t *testing.T) {
	t.Parallel()
	a := countPlan("doors", "# Stream — Other doors\n- [ ] one\n")
	b := countPlan("tenants", "# Stream — Several tenants\n- [x] one\n- [ ] two\n")
	if a.Name != "doors" || b.Name != "tenants" {
		t.Fatalf("names %q and %q: two stream files must stay apart", a.Name, b.Name)
	}
}

func TestPlanProgressReadsTheReadmeOfAPlanFolder(t *testing.T) {
	t.Parallel()
	enc := func(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }
	phase := enc("# Phase 6 — Verifier\n\n- [x] P1. done\n- [ ] P2. open\n")
	stream := enc("# Stream — Other doors\n- [ ] I8. open\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/app/contents/docs/plan":
			_, _ = fmt.Fprint(w, `[{"name":"doors","path":"docs/plan/doors","type":"dir"},`+
				`{"name":"notes","path":"docs/plan/notes","type":"dir"},`+
				`{"name":"phase-6","path":"docs/plan/phase-6","type":"dir"}]`)
		case "/repos/example/app/contents/docs/plan/phase-6/README.md":
			_, _ = fmt.Fprintf(w, `{"encoding":"base64","content":%q}`, phase)
		case "/repos/example/app/contents/docs/plan/doors/README.md":
			_, _ = fmt.Fprintf(w, `{"encoding":"base64","content":%q}`, stream)
		default: // docs/plan/notes has no README.md; a proposal file of a folder is never asked for
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	got, err := (&Client{BaseURL: srv.URL}).PlanProgress(t.Context(), "example/app")
	if err != nil {
		t.Fatal(err)
	}
	// Names are left to the naming tests: this one checks which files are read and counted.
	if len(got) != 2 || got[0].Done != 0 || got[0].Total != 1 || got[1].Done != 1 || got[1].Total != 2 {
		t.Fatalf("got %+v, want the doors folder with 0 of 1 and the phase folder with 1 of 2", got)
	}
}

func TestPlanProgressBoundsTheFetchesOfFoldersWithoutAReadme(t *testing.T) {
	t.Parallel()
	var listing []string
	for i := range maxPlanFiles + 10 {
		listing = append(listing, fmt.Sprintf(`{"name":"d%d","path":"docs/plan/d%d","type":"dir"}`, i, i))
	}
	var fetched atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/example/app/contents/docs/plan" {
			_, _ = fmt.Fprint(w, "["+strings.Join(listing, ",")+"]")
			return
		}
		fetched.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	got, err := (&Client{BaseURL: srv.URL}).PlanProgress(t.Context(), "example/app")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %+v, %v; want nothing and no error", got, err)
	}
	if n := fetched.Load(); n != maxPlanFiles {
		t.Fatalf("%d fetches for %d folders, want the cap of %d", n, len(listing), maxPlanFiles)
	}
}

func TestPlanProgressReportsAFolderReadmeThatCannotBeRead(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/example/app/contents/docs/plan" {
			_, _ = fmt.Fprint(w, `[{"name":"phase-6","path":"docs/plan/phase-6","type":"dir"}]`)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := (&Client{BaseURL: srv.URL}).PlanProgress(t.Context(), "example/app"); err == nil {
		t.Fatal("a server error on a folder's README.md must not read as a folder without a plan")
	}
}
