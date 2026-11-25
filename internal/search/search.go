// Package search is knowledge search over book.documents (design section 13). Full text
// always runs. Vector search runs only when an Embedder is set and the database has the
// pgvector column; the two lists are then fused with Reciprocal Rank Fusion. With no
// embedder nothing is embedded and nothing leaves the machine.
package search

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Embedder turns a text into a vector. The model that makes it is stored with each row, so
// a change of model shows which rows need a new embedding.
type Embedder interface {
	Embed(ctx context.Context, text string) (vec []float32, model string, err error)
}

// Index searches and fills the documents table.
type Index struct {
	Pool     *pgxpool.Pool
	Embedder Embedder // nil: full text only
}

// Query asks for documents. Visible lists the visibility levels the caller may read: a
// Builder gets only "all", Lead and Verifier also "lead_verifier".
type Query struct {
	Tenant            string
	Text              string
	Kinds             []string // empty: every kind
	Visible           []string
	IncludeSuperseded bool
	Limit             int // default 10
}

// Hit is one result with its quote, source and date (design section 13).
type Hit struct {
	ID       int64
	Kind     string
	Title    string
	Source   string
	Quote    string
	QuotedAt time.Time
	Status   string // proposed means unverified
}

// Document is a row to index.
type Document struct {
	Tenant, Kind, Source, Title, Body, Visibility, Status string
	QuotedAt                                              time.Time
}

// rrfK is the constant of Reciprocal Rank Fusion; 60 is the value of the original paper.
const rrfK = 60

// Fuse merges ranked lists with Reciprocal Rank Fusion: a document scores the sum of
// 1/(k+rank) over the lists it appears in. It needs no model and no score calibration.
func Fuse(lists ...[]Hit) []Hit {
	score := map[int64]float64{}
	first := map[int64]Hit{}
	for _, l := range lists {
		for i, h := range l {
			score[h.ID] += 1 / float64(rrfK+i+1)
			if _, ok := first[h.ID]; !ok {
				first[h.ID] = h
			}
		}
	}
	out := make([]Hit, 0, len(first))
	for _, h := range first {
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b Hit) int {
		switch {
		case score[a.ID] > score[b.ID]:
			return -1
		case score[a.ID] < score[b.ID]:
			return 1
		}
		return int(a.ID - b.ID)
	})
	return out
}

const selectHit = `SELECT id, kind, title, source_url, left(body, 300), coalesce(quoted_at, created_at), status FROM book.documents`

const filter = ` WHERE tenant = $1 AND visibility = ANY($2) AND ($3::text[] IS NULL OR kind = ANY($3)) AND ($4 OR status <> 'superseded')`

// Search runs full text and, when configured, vector search, and returns at most Limit hits.
func (ix *Index) Search(ctx context.Context, q Query) ([]Hit, error) {
	if strings.TrimSpace(q.Text) == "" {
		return nil, nil
	}
	if q.Limit <= 0 {
		q.Limit = 10
	}
	var kinds []string
	if len(q.Kinds) > 0 {
		kinds = q.Kinds
	}
	text, err := ix.query(ctx, selectHit+filter+
		` AND tsv @@ (websearch_to_tsquery('russian', $5) || websearch_to_tsquery('english', $5))
		 ORDER BY ts_rank(tsv, websearch_to_tsquery('russian', $5) || websearch_to_tsquery('english', $5)) DESC, id LIMIT $6`,
		q.Tenant, q.Visible, kinds, q.IncludeSuperseded, q.Text, q.Limit)
	if err != nil {
		return nil, err
	}
	if ix.Embedder == nil || !ix.hasVectors(ctx) {
		return text, nil
	}
	vec, model, err := ix.Embedder.Embed(ctx, q.Text)
	if err != nil {
		// A broken embedder must not take search down: full text still answers.
		return text, nil //nolint:nilerr // degrade to full text on purpose
	}
	near, err := ix.query(ctx, selectHit+filter+` AND embedding IS NOT NULL AND embedding_model = $5
		 ORDER BY embedding <=> $6::vector LIMIT $7`,
		q.Tenant, q.Visible, kinds, q.IncludeSuperseded, model, vectorText(vec), q.Limit)
	if err != nil {
		return text, nil //nolint:nilerr // degrade to full text on purpose
	}
	fused := Fuse(text, near)
	if len(fused) > q.Limit {
		fused = fused[:q.Limit]
	}
	return fused, nil
}

func (ix *Index) query(ctx context.Context, sql string, args ...any) ([]Hit, error) {
	rows, err := ix.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Kind, &h.Title, &h.Source, &h.Quote, &h.QuotedAt, &h.Status); err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return out, nil
}

// hasVectors reports whether the database has the pgvector column.
func (ix *Index) hasVectors(ctx context.Context) bool {
	var ok bool
	err := ix.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'book' AND table_name = 'documents' AND column_name = 'embedding')`).Scan(&ok)
	return err == nil && ok
}

// Add stores a document. With an embedder and the pgvector column it also stores the
// embedding and the model that made it; a failed embedding leaves the row searchable by
// full text and is not an error.
func (ix *Index) Add(ctx context.Context, d Document) (int64, error) {
	if d.Visibility == "" {
		d.Visibility = "all"
	}
	if d.Status == "" {
		d.Status = "active"
	}
	var quoted *time.Time
	if !d.QuotedAt.IsZero() {
		quoted = &d.QuotedAt
	}
	var id int64
	err := ix.Pool.QueryRow(ctx, `INSERT INTO book.documents (tenant, kind, source_url, title, body, quoted_at, visibility, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		d.Tenant, d.Kind, d.Source, d.Title, d.Body, quoted, d.Visibility, d.Status).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("search: add: %w", err)
	}
	if ix.Embedder == nil || !ix.hasVectors(ctx) {
		return id, nil
	}
	vec, model, err := ix.Embedder.Embed(ctx, d.Title+"\n"+d.Body)
	if err != nil {
		return id, nil //nolint:nilerr // the row stays searchable by full text
	}
	if _, err := ix.Pool.Exec(ctx, `UPDATE book.documents SET embedding = $2::vector, embedding_model = $3 WHERE id = $1`, id, vectorText(vec), model); err != nil {
		return id, fmt.Errorf("search: embedding of %d: %w", id, err)
	}
	return id, nil
}

// vectorText is the text form pgvector reads: [1,2,3].
func vectorText(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
