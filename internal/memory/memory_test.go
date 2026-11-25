package memory

import (
	"errors"
	"strings"
	"testing"
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
