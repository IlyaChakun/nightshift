package book

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Migrations are expand-only (AGENTS.md, internal/data/book): no drops, renames or type changes.
var forbidden = regexp.MustCompile(`(?i)\b(DROP\s+(TABLE|COLUMN|SCHEMA|INDEX|VIEW|TYPE|FUNCTION|TRIGGER|CONSTRAINT)|RENAME\s+(TO|COLUMN)|ALTER\s+COLUMN\s+\w+\s+(SET\s+DATA\s+)?TYPE)\b`)

func TestMigrationsAreExpandOnly(t *testing.T) {
	t.Parallel()
	files, err := fs.Glob(Migrations(), "*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	for _, name := range files {
		body, err := fs.ReadFile(Migrations(), name)
		if err != nil {
			t.Fatal(err)
		}
		sql := string(body)
		if !strings.Contains(sql, "-- +goose Up") {
			t.Errorf("%s: missing '-- +goose Up'", name)
		}
		if strings.Contains(sql, "-- +goose Down") {
			t.Errorf("%s: expand-only migrations have no Down section", name)
		}
		for i, line := range strings.Split(sql, "\n") {
			code, _, _ := strings.Cut(line, "--")
			if forbidden.MatchString(code) {
				t.Errorf("%s:%d: not expand-only: %q", name, i+1, strings.TrimSpace(code))
			}
		}
	}
}

func TestForbiddenPattern(t *testing.T) {
	t.Parallel()
	for sql, want := range map[string]bool{
		"DROP TABLE book.tasks":                         true,
		"ALTER TABLE t DROP COLUMN c":                   true,
		"ALTER TABLE t RENAME COLUMN a TO b":            true,
		"ALTER TABLE t ALTER COLUMN c TYPE bigint":      true,
		"ALTER TABLE t ALTER COLUMN c SET DATA TYPE x":  true,
		"ALTER TABLE t ADD COLUMN c text":               false,
		"CREATE TABLE book.drops (id bigint)":           false,
		"CREATE INDEX tasks_state ON book.tasks(state)": false,
	} {
		if got := forbidden.MatchString(sql); got != want {
			t.Errorf("%q: forbidden=%v, want %v", sql, got, want)
		}
	}
}
