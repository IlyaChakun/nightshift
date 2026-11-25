// Package memory keeps the project memory page: facts that would otherwise be asked of the
// owner again (design section 13). The page is small, capped, and written only by compare-and-swap.
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

const (
	// MaxEntries and MaxBytes cap the page, so it can go whole into a prompt.
	MaxEntries = 40
	MaxBytes   = 4096
	// DefaultTTL is how long an entry lives unless the owner pinned it.
	DefaultTTL = 90 * 24 * time.Hour
)

var (
	// ErrStale means the write named a revision that is no longer the page's.
	ErrStale = errors.New("memory: the page changed since it was read; read it again")
	// ErrFull means the page would pass its cap.
	ErrFull = errors.New("memory: page full: merge or drop an entry")
	// ErrEntry means an entry is not acceptable.
	ErrEntry = errors.New("memory: bad entry")
)

// Entry is one line of the page. A zero ExpiresAt means the owner pinned it.
type Entry struct {
	Key       string    `json:"key"`
	Text      string    `json:"text"`
	Source    string    `json:"source"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

// Page is the tenant's page at one revision. A tenant without a page has revision 0.
type Page struct {
	Revision int64
	Entries  []Entry
}

// Size is the bytes the page takes against MaxBytes.
func Size(entries []Entry) int {
	n := 0
	for _, e := range entries {
		n += len(e.Key) + len(e.Text) + len(e.Source)
	}
	return n
}

// ValidSource reports whether a source may write to the page: a message of the owner or a
// file of the tenant repository. Text from tickets, the web or a preview never qualifies.
func ValidSource(s string) bool {
	for _, p := range []string{"owner:", "repo:"} {
		if rest, ok := strings.CutPrefix(s, p); ok && strings.TrimSpace(rest) != "" {
			return true
		}
	}
	return false
}

// Validate checks the entries of a page against the shape and the caps.
func Validate(entries []Entry) error {
	if len(entries) > MaxEntries {
		return ErrFull
	}
	seen := map[string]bool{}
	for _, e := range entries {
		switch {
		case strings.TrimSpace(e.Key) == "" || strings.TrimSpace(e.Text) == "":
			return fmt.Errorf("%w: key and text are required", ErrEntry)
		case strings.ContainsAny(e.Key+e.Text, "\r\n"):
			return fmt.Errorf("%w: an entry is one line (%q)", ErrEntry, e.Key)
		case !ValidSource(e.Source):
			return fmt.Errorf("%w: source %q is not an owner message or a repository file", ErrEntry, e.Source)
		case seen[e.Key]:
			return fmt.Errorf("%w: key %q appears twice", ErrEntry, e.Key)
		}
		seen[e.Key] = true
	}
	if Size(entries) > MaxBytes {
		return ErrFull
	}
	return nil
}

// Store reads and writes pages in the book.
type Store struct{ Pool *pgxpool.Pool }

// Read returns the tenant's page; a tenant without one gets revision 0 and no entries.
func (s Store) Read(ctx context.Context, tenant string) (Page, error) {
	row, err := bookdb.New(s.Pool).GetMemoryPage(ctx, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, nil
	}
	if err != nil {
		return Page{}, fmt.Errorf("memory: read: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(row.Entries, &entries); err != nil {
		return Page{}, fmt.Errorf("memory: read: %w", err)
	}
	return Page{Revision: row.Revision, Entries: entries}, nil
}

// Write replaces the page's entries, if the page is still at revision rev (0 for a new page).
// The change and its event commit together; a stale revision or an over-cap page is refused.
func (s Store) Write(ctx context.Context, tenant string, rev int64, entries []Entry, actor, why string) error {
	if err := Validate(entries); err != nil {
		return err
	}
	if entries == nil {
		entries = []Entry{}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := bookdb.New(tx)
	var n int64
	if rev == 0 {
		n, err = q.CreateMemoryPage(ctx, bookdb.CreateMemoryPageParams{Tenant: tenant, Entries: raw})
	} else {
		n, err = q.UpdateMemoryPage(ctx, bookdb.UpdateMemoryPageParams{Tenant: tenant, Revision: rev, Entries: raw})
	}
	if err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	if n == 0 {
		return ErrStale
	}
	payload, err := json.Marshal(map[string]any{"revision": rev + 1, "entries": len(entries), "why": why})
	if err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: tenant, TaskID: pgtype.Int8{}, Kind: "memory.written", Actor: actor, Payload: payload,
	}); err != nil {
		return fmt.Errorf("memory: write: %w", err)
	}
	return tx.Commit(ctx)
}

// Remember adds or replaces (by key) one entry the owner wrote, reading the page first.
// The key is derived from the text, so the same sentence twice is one entry.
func (s Store) Remember(ctx context.Context, tenant, text, actor string, now time.Time) error {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return fmt.Errorf("%w: nothing to remember", ErrEntry)
	}
	page, err := s.Read(ctx, tenant)
	if err != nil {
		return err
	}
	e := Entry{Key: Key(text), Text: text, Source: "owner:" + actor, ExpiresAt: now.Add(DefaultTTL)}
	entries := make([]Entry, 0, len(page.Entries)+1)
	for _, old := range page.Entries {
		if old.Key != e.Key {
			entries = append(entries, old)
		}
	}
	return s.Write(ctx, tenant, page.Revision, append(entries, e), actor, "remember")
}

// Key is a short stable name for a text: its first words, lower case.
func Key(text string) string {
	words := strings.Fields(strings.ToLower(text))
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Join(words, "-")
}
