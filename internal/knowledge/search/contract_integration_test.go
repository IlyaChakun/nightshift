package search_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/knowledge/search"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// conceptEmbedder places a text by the concepts it names, so a query can find a document
// that shares no word with it: "deadline" lands next to "timeout". Every vector is non-zero,
// because pgvector's cosine distance of a zero vector is NaN.
type conceptEmbedder struct{}

func (conceptEmbedder) Embed(_ context.Context, text string) ([]float32, string, error) {
	text = strings.ToLower(text)
	v := []float32{0.01, 0.01, 0.01}
	switch {
	case strings.Contains(text, "timeout"), strings.Contains(text, "deadline"):
		v[0] = 1
	case strings.Contains(text, "export"):
		v[1] = 1
	default:
		v[2] = 1
	}
	return v, "concept-1", nil
}

// TestRetrievalContractFullText and TestRetrievalContractVectors are the retrieval contract
// (docs/plan/architecture/ item A7): whatever path answers, a hit belongs to the asking tenant,
// is visible to the asking role, is not superseded unless asked, and carries its provenance
// (source, date, status).
func TestRetrievalContractFullText(t *testing.T) {
	runContract(t, &search.Index{Pool: pgtest.Start(t).ServicePool(t)}, []string{"timeout"})
}

func TestRetrievalContractVectors(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	if !hasVectorColumn(t, pool) {
		// The pgtest image has pgvector, so a missing column there is a broken migration.
		if os.Getenv(pgtest.ExternalEnv) == "" { //nolint:forbidigo // a test-only switch, not configuration
			t.Fatal("the pgtest image has pgvector but book.documents has no embedding column")
		}
		t.Skip("this external Postgres has no pgvector")
	}
	// "deadline" matches no word of any document: only the vector path finds them.
	runContract(t, &search.Index{Pool: pool, Embedder: conceptEmbedder{}}, []string{"timeout", "deadline"})
}

// corpus is the documents of the contract: four of example-solo about timeouts, two about
// something else, and two of example-team. The vector path ranks every row, so the queries set
// a limit below the corpus: only an embedder that ranks by meaning keeps the others out.
type corpus struct {
	ix                          *search.Index
	quoted                      time.Time
	mine, proposed, hidden, old int64
	foreign                     []int64
}

var everything = []string{"all", "lead_verifier"}

func runContract(t *testing.T, ix *search.Index, texts []string) {
	c := seed(t, ix)
	for _, text := range texts {
		c.filters(t, text)
		c.provenance(t, text)
	}
}

func seed(t *testing.T, ix *search.Index) *corpus {
	t.Helper()
	c := &corpus{ix: ix, quoted: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	add := func(d search.Document) int64 {
		t.Helper()
		id, err := ix.Add(t.Context(), d)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	c.mine = add(search.Document{Tenant: "example-solo", Kind: "learning", Source: "docs/learnings.md#retries", Title: "Retries",
		Body: "Calls to the export service need a timeout.", QuotedAt: c.quoted})
	c.proposed = add(search.Document{Tenant: "example-solo", Kind: "note", Source: "task:7", Title: "Guess",
		Body: "Maybe the timeout should be longer.", Status: "proposed"})
	c.hidden = add(search.Document{Tenant: "example-solo", Kind: "scenario", Source: "task:7", Title: "Hidden",
		Body: "Hidden check of the timeout.", Visibility: "lead_verifier"})
	c.old = add(search.Document{Tenant: "example-solo", Kind: "decision", Source: "docs/decisions.md", Title: "Old",
		Body: "The first timeout decision.", Status: "superseded"})
	add(search.Document{Tenant: "example-solo", Kind: "learning", Source: "docs/learnings.md#csv", Title: "CSV", Body: "The export writes CSV."})
	add(search.Document{Tenant: "example-solo", Kind: "decision", Source: "docs/decisions.md", Title: "Export", Body: "Export runs nightly."})
	c.foreign = []int64{
		add(search.Document{Tenant: "example-team", Kind: "learning", Source: "team", Title: "Team", Body: "Their timeout."}),
		add(search.Document{Tenant: "example-team", Kind: "scenario", Source: "team", Title: "Team hidden", Body: "Their timeout check.", Visibility: "lead_verifier"}),
	}
	return c
}

func (c *corpus) hits(t *testing.T, q search.Query) []search.Hit {
	t.Helper()
	hits, err := c.ix.Search(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func (c *corpus) ids(t *testing.T, q search.Query) []int64 {
	t.Helper()
	var ids []int64
	for _, h := range c.hits(t, q) {
		ids = append(ids, h.ID)
	}
	return ids
}

// filters: a hit belongs to the asking tenant and is visible to the asking role; a query that
// names no tenant or no visibility gets nothing, not everything.
func (c *corpus) filters(t *testing.T, text string) {
	t.Helper()
	builder := c.ids(t, search.Query{Tenant: "example-solo", Text: text, Visible: []string{"all"}, Limit: 2})
	if want := []int64{c.mine, c.proposed}; !sameIDs(builder, want) {
		t.Errorf("builder view of %q: %v, want %v", text, builder, want)
	}
	full := c.ids(t, search.Query{Tenant: "example-solo", Text: text, Visible: everything, IncludeSuperseded: true, Limit: 4})
	if want := []int64{c.mine, c.proposed, c.hidden, c.old}; !sameIDs(full, want) {
		t.Errorf("full view of %q: %v, want %v (example-team's are %v)", text, full, want, c.foreign)
	}
	if got := c.ids(t, search.Query{Text: text, Visible: everything, IncludeSuperseded: true}); len(got) != 0 {
		t.Errorf("%q with no tenant: %v", text, got)
	}
	if got := c.ids(t, search.Query{Tenant: "example-solo", Text: text}); len(got) != 0 {
		t.Errorf("%q with no visibility: %v", text, got)
	}
	theirs := c.ids(t, search.Query{Tenant: "example-team", Text: text, Visible: everything})
	if !sameIDs(theirs, c.foreign) {
		t.Errorf("example-team's view of %q: %v, want %v", text, theirs, c.foreign)
	}
}

// provenance: every hit carries its source, quote, date and status; an unverified document
// reads as proposed.
func (c *corpus) provenance(t *testing.T, text string) {
	t.Helper()
	byID := map[int64]search.Hit{}
	for _, h := range c.hits(t, search.Query{Tenant: "example-solo", Text: text, Visible: everything, IncludeSuperseded: true, Limit: 4}) {
		byID[h.ID] = h
		if h.Source == "" || h.Quote == "" || h.QuotedAt.IsZero() {
			t.Errorf("hit %d has no provenance: %+v", h.ID, h)
		}
	}
	if h := byID[c.mine]; !h.QuotedAt.Equal(c.quoted) || h.Source != "docs/learnings.md#retries" || h.Status != "active" || h.Kind != "learning" {
		t.Errorf("hit %d lost its source, date, kind or status: %+v", c.mine, h)
	}
	if h := byID[c.proposed]; h.Status != "proposed" {
		t.Errorf("an unverified document reads as %q, want proposed", h.Status)
	}
	if h := byID[c.old]; h.Status != "superseded" {
		t.Errorf("a superseded document reads as %q", h.Status)
	}
}

func sameIDs(got, want []int64) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}

func hasVectorColumn(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'book' AND table_name = 'documents' AND column_name = 'embedding')`).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}
