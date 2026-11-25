package dispatch

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

func ev(kind, actor, payload string) bookdb.BookEvent {
	return bookdb.BookEvent{Kind: kind, Actor: actor, Payload: []byte(payload)}
}

func TestHandoffText(t *testing.T) {
	stored := ev(eventHandoff, "builder", `{"done":["parser"],"pending":["tests"],"decisions":["use RE2"]}`)
	newer := ev(eventHandoff, "builder", `{"done":["parser","tests"],"pending":[]}`)
	tests := []struct {
		name   string
		events []bookdb.BookEvent
		role   string
		want   []string
		not    []string
	}{
		{"latest stored handoff wins", []bookdb.BookEvent{stored, newer}, "builder", []string{"Done:\n- parser\n- tests"}, []string{"use RE2", "Pending"}},
		{"another role's handoff is not shown", []bookdb.BookEvent{stored}, "lead", nil, []string{"parser"}},
		{"none written: minimal one from events", []bookdb.BookEvent{ev("task_created", "chat", `{}`), ev("job.started", "system", `{}`), ev("builder.returned", "system", `{}`)}, "builder", []string{"No handoff was written", "job.started", "builder.returned"}, []string{"task_created"}},
		{"nothing to hand over", []bookdb.BookEvent{ev("task_created", "chat", `{}`)}, "builder", nil, []string{"No handoff"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handoffText(tt.events, tt.role)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in %q", n, got)
				}
			}
		})
	}
}

func TestBuilderPromptCarriesTheHandoff(t *testing.T) {
	p := builderPrompt(bookdb.BookTask{Title: "T"}, nil, "", "Done:\n- parser")
	if !strings.Contains(p, "Previous session, unverified") || !strings.Contains(p, "- parser") {
		t.Fatalf("prompt: %q", p)
	}
	if strings.Contains(builderPrompt(bookdb.BookTask{Title: "T"}, nil, "", ""), "Previous session") {
		t.Fatal("a task without a handoff must not mention one")
	}
}
