package dispatch

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

func leadEv(kind, text string) bookdb.BookEvent {
	return bookdb.BookEvent{Kind: kind, Payload: []byte(`{"text":"` + text + `"}`)}
}

func TestRejectionsAfterOwnerResetOnANewOwnerMessage(t *testing.T) {
	tests := []struct {
		name   string
		events []bookdb.BookEvent
		want   int
	}{
		{"none", []bookdb.BookEvent{leadEv("task_created", "idea")}, 0},
		{"one", []bookdb.BookEvent{leadEv("task_created", "idea"), leadEv(eventLeadRejected, "x")}, 1},
		{"reset", []bookdb.BookEvent{leadEv("task_created", "idea"), leadEv(eventLeadRejected, "x"), leadEv("owner_message", "go on")}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectionsAfterOwner(tt.events); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCorrectionNoteOnlyWhileTheRejectionIsTheLatestWord(t *testing.T) {
	rejected := []bookdb.BookEvent{leadEv("task_created", "idea"), leadEv(eventLeadRejected, "the text is too long")}
	if got := correctionNote(rejected); !strings.Contains(got, "the text is too long") {
		t.Fatalf("no correction in %q", got)
	}
	for _, after := range []bookdb.BookEvent{leadEv(eventLeadReply, "ok"), leadEv("owner_message", "go on")} {
		if got := correctionNote(append(slicesClone(rejected), after)); got != "" {
			t.Fatalf("a stale correction after %s: %q", after.Kind, got)
		}
	}
	garbled := []bookdb.BookEvent{leadEv("task_created", "idea"), {Kind: eventLeadRejected, Payload: []byte(`not json`)}}
	if got := correctionNote(garbled); got != "" {
		t.Fatalf("garbled payload gave %q", got)
	}
}

func slicesClone(e []bookdb.BookEvent) []bookdb.BookEvent {
	return append([]bookdb.BookEvent(nil), e...)
}
