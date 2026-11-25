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
	big := []Entry{entry("a", strings.Repeat("x", MaxBytes), "owner:1")}
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
	if got := Key("  Run the tests with  make test please, every time "); got != "run-the-tests-with-make-test" {
		t.Fatal(got)
	}
}
