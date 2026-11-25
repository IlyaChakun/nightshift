-- +goose Up
-- Knowledge the factory searches (design section 13): decisions, lessons, task outcomes,
-- notes. Full text always works; the embedding column exists from the start so that turning
-- embeddings on later needs no migration of live rows.
CREATE TABLE book.documents (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant          text        NOT NULL,
    kind            text        NOT NULL,
    source_url      text        NOT NULL DEFAULT '',
    title           text        NOT NULL,
    body            text        NOT NULL,
    quoted_at       timestamptz,
    visibility      text        NOT NULL DEFAULT 'all',
    status          text        NOT NULL DEFAULT 'active',
    superseded_by   bigint      REFERENCES book.documents (id),
    provenance      jsonb       NOT NULL DEFAULT '{}',
    tsv             tsvector GENERATED ALWAYS AS (
        to_tsvector('russian', title || ' ' || body) || to_tsvector('english', title || ' ' || body)
    ) STORED,
    embedding_model text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT documents_visibility CHECK (visibility IN ('all', 'lead_verifier')),
    CONSTRAINT documents_status CHECK (status IN ('active', 'proposed', 'superseded'))
);
CREATE INDEX documents_tsv ON book.documents USING gin (tsv);
CREATE INDEX documents_tenant_kind ON book.documents (tenant, kind, status);

-- The vector column needs the pgvector extension. An image without it still gets a working
-- full-text search, and search.embedder stays unset (the search code checks for the column).
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'vector') THEN
        CREATE EXTENSION IF NOT EXISTS vector;
        ALTER TABLE book.documents ADD COLUMN embedding vector;
    END IF;
END
$$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE ON book.documents TO nightshift;
