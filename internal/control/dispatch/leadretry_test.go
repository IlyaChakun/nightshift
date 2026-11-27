package dispatch

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

func leadEv(kind, text string) bookdb.BookEvent {
	return bookdb.BookEvent{Kind: kind, Payload: []byte(`{"text":"` + text + `"}`)}
}

func ofJob7(e bookdb.BookEvent) bookdb.BookEvent {
	e.JobID = pgtype.Int8{Int64: 7, Valid: true}
	return e
}
func ofJob6(e bookdb.BookEvent) bookdb.BookEvent {
	e.JobID = pgtype.Int8{Int64: 6, Valid: true}
	return e
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
		{"the current job's own rejection is not an earlier one", []bookdb.BookEvent{leadEv("task_created", "idea"), ofJob7(leadEv(eventLeadRejected, "x"))}, 0},
		{"an earlier job's rejection counts", []bookdb.BookEvent{leadEv("task_created", "idea"), ofJob6(leadEv(eventLeadRejected, "x"))}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectionsAfterOwner(tt.events, 7); got != tt.want {
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
