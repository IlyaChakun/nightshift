package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/knowledge/search"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

type brokenEmbedder struct{}

func (brokenEmbedder) Embed(context.Context, string) ([]float32, string, error) {
	return nil, "", errors.New("embedder is down")
}

func TestFullTextSearch(t *testing.T) {
	ix := &search.Index{Pool: pgtest.Start(t).ServicePool(t)}
	ctx := t.Context()
	add := func(d search.Document) int64 {
		t.Helper()
		if d.Tenant == "" {
			d.Tenant = "example-solo"
		}
		id, err := ix.Add(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	timeout := add(search.Document{Kind: "learning", Title: "Retries", Body: "Calls to the export service need a timeout and a retry budget."})
	ru := add(search.Document{Kind: "decision", Title: "Выгрузка", Body: "Решили выгружать данные в формате CSV, а не JSON."})
	hidden := add(search.Document{Kind: "decision", Title: "Hidden", Body: "Hidden scenario for the export timeout.", Visibility: "lead_verifier"})
	old := add(search.Document{Kind: "decision", Title: "Old", Body: "The old timeout decision.", Status: "superseded"})
	add(search.Document{Tenant: "other", Kind: "learning", Title: "Other tenant", Body: "timeout"})

	got, err := ix.Search(ctx, search.Query{Tenant: "example-solo", Text: "timeout", Visible: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != timeout {
		t.Fatalf("builder view of %q: %+v", "timeout", got)
	}
	if got, _ = ix.Search(ctx, search.Query{Tenant: "example-solo", Text: "timeout", Visible: []string{"all", "lead_verifier"}, IncludeSuperseded: true}); len(got) != 3 {
		t.Fatalf("full view found %d, want 3 (%d, %d hidden from builders)", len(got), hidden, old)
	}
	if got, _ = ix.Search(ctx, search.Query{Tenant: "example-solo", Text: "выгрузка CSV", Visible: []string{"all"}}); len(got) != 1 || got[0].ID != ru || got[0].Quote == "" {
		t.Fatalf("russian query: %+v", got)
	}
	if got, _ = ix.Search(ctx, search.Query{Tenant: "example-solo", Text: "timeout", Kinds: []string{"decision"}, Visible: []string{"all"}}); len(got) != 0 {
		t.Fatalf("kind filter: %+v", got)
	}
	if got, _ = ix.Search(ctx, search.Query{Tenant: "nobody", Text: "timeout", Visible: []string{"all"}}); len(got) != 0 {
		t.Fatalf("another tenant sees %+v", got)
	}
}

func TestBrokenEmbedderFallsBackToFullText(t *testing.T) {
	ix := &search.Index{Pool: pgtest.Start(t).ServicePool(t), Embedder: brokenEmbedder{}}
	if _, err := ix.Add(t.Context(), search.Document{Tenant: "t", Kind: "note", Title: "x", Body: "needle in text"}); err != nil {
		t.Fatalf("a failed embedding must not fail Add: %v", err)
	}
	got, err := ix.Search(t.Context(), search.Query{Tenant: "t", Text: "needle", Visible: []string{"all"}})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}
