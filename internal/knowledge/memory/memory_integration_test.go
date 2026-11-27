package memory_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/knowledge/memory"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestPageWritesAreCompareAndSwap(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	now := time.Now()

	if p, err := s.Read(ctx, "example-solo"); err != nil || p.Revision != 0 || len(p.Entries) != 0 {
		t.Fatalf("empty page = %+v, %v", p, err)
	}
	if err := s.Remember(ctx, "example-solo", "tests run with make test", "U1", now); err != nil {
		t.Fatal(err)
	}
	p, err := s.Read(ctx, "example-solo")
	if err != nil || p.Revision != 1 || len(p.Entries) != 1 || p.Entries[0].Source != "owner:U1" {
		t.Fatalf("page = %+v, %v", p, err)
	}
	// The same sentence twice is one entry, and the revision moves.
	if err := s.Remember(ctx, "example-solo", "tests run with  make test", "U1", now); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.Read(ctx, "example-solo"); p.Revision != 2 || len(p.Entries) != 1 {
		t.Fatalf("after repeat: %+v", p)
	}
	// A writer that read revision 1 is refused, and the page keeps its entries.
	stale := []memory.Entry{{Key: "x", Text: "y", Source: "owner:U2"}}
	if err := s.Write(ctx, "example-solo", 1, stale, "U2", "test"); !errors.Is(err, memory.ErrStale) {
		t.Fatalf("stale write = %v", err)
	}
	// A second creator of a new page is stale too.
	if err := s.Write(ctx, "example-solo", 0, stale, "U2", "test"); !errors.Is(err, memory.ErrStale) {
		t.Fatalf("create over an existing page = %v", err)
	}
	if p, _ = s.Read(ctx, "example-solo"); len(p.Entries) != 1 || p.Entries[0].Key == "x" {
		t.Fatalf("a refused write changed the page: %+v", p)
	}
}

func TestPageOverTheCapIsNotExtended(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	var entries []memory.Entry
	for i := range memory.MaxEntries {
		entries = append(entries, memory.Entry{Key: string(rune('a'+i%26)) + string(rune('a'+i/26)), Text: "fact", Source: "owner:U1"})
	}
	if err := s.Write(ctx, "example-solo", 0, entries, "U1", "fill"); err != nil {
		t.Fatal(err)
	}
	err := s.Remember(ctx, "example-solo", "one more fact", "U1", time.Now())
	if !errors.Is(err, memory.ErrFull) {
		t.Fatalf("Remember on a full page = %v", err)
	}
	if p, _ := s.Read(ctx, "example-solo"); len(p.Entries) != memory.MaxEntries || p.Revision != 1 {
		t.Fatalf("page changed: rev %d, %d entries", p.Revision, len(p.Entries))
	}
}

func TestTwoWritersOneWins(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			errs[i] = s.Write(ctx, "example-solo", 0, []memory.Entry{{Key: "k", Text: "v", Source: "owner:U1"}}, "U1", "race")
		})
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("exactly one writer must win: %v, %v", errs[0], errs[1])
	}
}

func TestANewFactDoesNotReplaceAnotherWithTheSameWords(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	for _, text := range []string{"tests run with make test in CI", "tests run with make test locally"} {
		if err := s.Remember(ctx, "example-solo", text, "U1", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if p, _ := s.Read(ctx, "example-solo"); len(p.Entries) != 2 {
		t.Fatalf("entries = %+v", p.Entries)
	}
}

func TestPruneDropsExpiredAndRepeatedEntriesAndRecordsEach(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	now := time.Now()
	entries := []memory.Entry{
		{Key: "a", Text: "Tests run with make test", Source: "owner:U1", ExpiresAt: now.Add(time.Hour)},
		{Key: "b", Text: "tests  run with MAKE test", Source: "owner:U2", ExpiresAt: now.Add(time.Hour)}, // repeats a
		{Key: "c", Text: "old fact", Source: "owner:U1", ExpiresAt: now.Add(-time.Hour)},
		{Key: "d", Text: "pinned fact", Source: "repo:AGENTS.md"},
	}
	if err := s.Write(ctx, "example-solo", 0, entries, "U1", "seed"); err != nil {
		t.Fatal(err)
	}
	dropped, err := s.Prune(ctx, "example-solo", now, nil)
	if err != nil || len(dropped) != 2 || dropped[0] != "expired:c" || dropped[1] != "duplicate:b" {
		t.Fatalf("dropped %v, %v", dropped, err)
	}
	p, _ := s.Read(ctx, "example-solo")
	if p.Revision != 2 || len(p.Entries) != 2 || p.Entries[0].Key != "a" || p.Entries[1].Key != "d" {
		t.Fatalf("page after prune: %+v", p)
	}
	var why string
	if err := db.Owner.QueryRowContext(ctx, `SELECT payload->>'why' FROM book.events WHERE kind = 'memory.written' ORDER BY id DESC LIMIT 1`).Scan(&why); err != nil || why != "prune: expired:c, duplicate:b" {
		t.Fatalf("event why %q, %v", why, err)
	}
	// Each drop is an event of its own, with the text and the reason.
	rows, err := db.Owner.QueryContext(ctx, `SELECT payload->>'key', payload->>'reason', payload->>'text' FROM book.events WHERE kind = 'memory.dropped' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var k, r, x string
		if err := rows.Scan(&k, &r, &x); err != nil {
			t.Fatal(err)
		}
		got = append(got, k+"/"+r+"/"+x)
	}
	if len(got) != 2 || got[0] != "c/expired/old fact" || got[1] != "b/duplicate/tests  run with MAKE test" {
		t.Fatalf("drop events: %v", got)
	}
	// Nothing to drop: no write, the revision stays.
	if dropped, err := s.Prune(ctx, "example-solo", now, nil); err != nil || len(dropped) != 0 {
		t.Fatalf("second prune: %v, %v", dropped, err)
	}
	if p, _ = s.Read(ctx, "example-solo"); p.Revision != 2 {
		t.Fatalf("a prune with nothing to drop wrote: revision %d", p.Revision)
	}
}

// Of two entries with the same text the pinned one stays, else the one that lives longer, so a
// fact is never lost to a prune because a shorter-lived copy came first.
func TestPruneKeepsThePinnedOrLongerLivedOfTwoRepeats(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	now := time.Now()
	entries := []memory.Entry{
		{Key: "short", Text: "staging is blue", Source: "owner:U1", ExpiresAt: now.Add(24 * time.Hour)},
		{Key: "pinned", Text: "staging is blue", Source: "repo:AGENTS.md"},
		{Key: "x1", Text: "other fact", Source: "owner:U1", ExpiresAt: now.Add(24 * time.Hour)},
		{Key: "x2", Text: "other fact", Source: "owner:U1", ExpiresAt: now.Add(48 * time.Hour)},
	}
	if err := s.Write(ctx, "example-solo", 0, entries, "U1", "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(ctx, "example-solo", now, nil); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Read(ctx, "example-solo")
	if len(p.Entries) != 2 || p.Entries[0].Key != "pinned" || p.Entries[1].Key != "x2" {
		t.Fatalf("kept %+v", p.Entries)
	}
}

func TestRememberIgnoresExpiredEntriesAtTheCap(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	past := time.Now().Add(-time.Hour)
	var entries []memory.Entry
	for i := range memory.MaxEntries {
		entries = append(entries, memory.Entry{Key: string(rune('a'+i%26)) + string(rune('a'+i/26)), Text: "old", Source: "owner:U1", ExpiresAt: past})
	}
	if err := s.Write(ctx, "example-solo", 0, entries, "U1", "fill"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remember(ctx, "example-solo", "a fresh fact", "U1", time.Now()); err != nil {
		t.Fatalf("Remember over expired entries = %v", err)
	}
	if p, _ := s.Read(ctx, "example-solo"); len(p.Entries) != 1 {
		t.Fatalf("entries = %d, want only the fresh one", len(p.Entries))
	}
}

// An entry whose text the repository now states goes, and the reason names the file. A pinned entry, a repo-sourced one, a short
// text, a text the repository lacks and a failed read keep their entries.
func TestPruneDropsAnEntryThatMovedIntoTheRepository(t *testing.T) {
	db := pgtest.Start(t)
	s := memory.Store{Pool: db.ServicePool(t)}
	ctx := t.Context()
	now := time.Now()
	in := now.Add(time.Hour)
	entries := []memory.Entry{
		{Key: "moved", Text: "Tests run with make test on this machine", Source: "owner:U1", ExpiresAt: in},
		{Key: "cited", Text: "previews use port 8081", Source: "repo:docs/preview.md", ExpiresAt: in},
		{Key: "pinned", Text: "Tests run with make test on this machine, pinned", Source: "owner:U1"},
		{Key: "short", Text: "tabs", Source: "owner:U1", ExpiresAt: in},
		{Key: "absent", Text: "the owner prefers short answers", Source: "owner:U1", ExpiresAt: in},
	}
	if err := s.Write(ctx, "example-solo", 0, entries, "U1", "seed"); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"AGENTS.md":       "# Rules\n\ntests run with MAKE test\non this machine.\nTabs everywhere",
		"docs/preview.md": "Previews use port 8081.",
	}
	read := func(_ context.Context, p string) (string, bool, error) { b, ok := files[p]; return b, ok, nil }
	dropped, err := s.Prune(ctx, "example-solo", now, read)
	if err != nil || len(dropped) != 1 || dropped[0] != "in repo: AGENTS.md:moved" {
		t.Fatalf("dropped %v, %v", dropped, err)
	}
	// A read that fails keeps the entry.
	failing := func(context.Context, string) (string, bool, error) { return "", false, errors.New("mirror down") }
	if dropped, err := s.Prune(ctx, "example-solo", now, failing); err != nil || len(dropped) != 0 {
		t.Fatalf("a failed read dropped %v, %v", dropped, err)
	}
}
