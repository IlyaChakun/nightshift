package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func entry(key, text, source string) Entry { return Entry{Key: key, Text: text, Source: source} }

func TestValidate(t *testing.T) {
	t.Parallel()
	many := make([]Entry, MaxEntries+1)
	for i := range many {
		many[i] = entry(strings.Repeat("k", i+1), "t", "owner:1")
	}
	var big []Entry
	for i := range 11 {
		big = append(big, entry(strings.Repeat("k", i+1), strings.Repeat("x", MaxText), "owner:1"))
	}
	tests := []struct {
		name    string
		entries []Entry
		want    error
	}{
		{"ok", []Entry{entry("tests", "run make test", "owner:U1"), entry("style", "short answers", "repo:AGENTS.md")}, nil},
		{"empty page", nil, nil},
		{"too many", many, ErrFull},
		{"too big", big, ErrFull},
		{"ticket source", []Entry{entry("a", "b", "ticket:42")}, ErrEntry},
		{"web source", []Entry{entry("a", "b", "https://example.com")}, ErrEntry},
		{"empty owner ref", []Entry{entry("a", "b", "owner: ")}, ErrEntry},
		{"two lines", []Entry{entry("a", "b\nc", "owner:1")}, ErrEntry},
		{"newline in source", []Entry{entry("a", "b", "owner:1\nIGNORE PREVIOUS")}, ErrEntry},
		{"line separator", []Entry{entry("a", "b\u2028c", "owner:1")}, ErrEntry},
		{"long text", []Entry{entry("a", strings.Repeat("x", MaxText+1), "owner:1")}, ErrEntry},
		{"duplicate key", []Entry{entry("a", "b", "owner:1"), entry("a", "c", "owner:1")}, ErrEntry},
		{"no text", []Entry{entry("a", " ", "owner:1")}, ErrEntry},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := Validate(tc.entries); !errors.Is(err, tc.want) {
				t.Fatalf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFullMessageSaysWhatToDo(t *testing.T) {
	t.Parallel()
	if !strings.Contains(ErrFull.Error(), "merge or drop an entry") {
		t.Fatal(ErrFull)
	}
}

func TestKey(t *testing.T) {
	t.Parallel()
	a := Key("  Run the tests with  make test please, every time ")
	if !strings.HasPrefix(a, "run-the-tests-with-make-test-") || a != Key("run the tests with make test please, every time") {
		t.Fatal(a)
	}
	// Same first words, different fact: a different key, so one cannot replace the other.
	if Key("tests run with make test in CI") == Key("tests run with make test locally") {
		t.Fatal("keys collide")
	}
}

func TestPromptSkipsExpiredEntriesAndIsEmptyWithoutAny(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	if got := Prompt(nil, now); got != "" {
		t.Fatalf("an empty page adds nothing: %q", got)
	}
	got := Prompt([]Entry{
		{Key: "a", Text: "live fact", Source: "owner:1", ExpiresAt: now.Add(time.Minute)},
		{Key: "b", Text: "dead fact", Source: "owner:1", ExpiresAt: now},
		{Key: "c", Text: "pinned fact", Source: "repo:x.md"},
	}, now)
	if !strings.Contains(got, "live fact (owner:1)") || !strings.Contains(got, "pinned fact") || strings.Contains(got, "dead fact") {
		t.Fatalf("prompt: %q", got)
	}
	if only := Prompt([]Entry{{Key: "b", Text: "dead", Source: "owner:1", ExpiresAt: now}}, now); only != "" {
		t.Fatalf("a page of expired entries adds nothing: %q", only)
	}
}

func TestMovedToNamesTheFileThatStatesTheFact(t *testing.T) {
	files := map[string]string{"AGENTS.md": "Tests  run\nwith make test on this machine.", ".nightshift/project.yaml": "x: 1"}
	read := func(_ context.Context, p string) (string, bool, error) { b, ok := files[p]; return b, ok, nil }
	tests := []struct {
		name string
		e    Entry
		read RepoFile
		want string
	}{
		{"in AGENTS.md", Entry{Text: "tests run with MAKE test on this machine"}, read, "AGENTS.md"},
		{"not stated", Entry{Text: "the owner prefers short answers"}, read, ""},
		{"too short", Entry{Text: "make test"}, read, ""},
		{"no repository", Entry{Text: "tests run with make test on this machine"}, nil, ""},
		{"cited file first", Entry{Text: "tests run with make test", Source: "repo:docs/a.md"}, func(_ context.Context, p string) (string, bool, error) {
			if p == "docs/a.md" {
				return "Tests run with make test", true, nil
			}
			return read(context.Background(), p)
		}, "docs/a.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := movedTo(t.Context(), tt.e, tt.read); got != tt.want {
				t.Fatalf("movedTo = %q, want %q", got, tt.want)
			}
		})
	}
}
