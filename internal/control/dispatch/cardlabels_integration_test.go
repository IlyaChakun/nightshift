package dispatch_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
)

const russianSpec = `{"title":"Выгрузка в CSV","goal":"Выгрузить отчёт в CSV.","prd_path":"docs/prd/csv.md",
 "non_goals":["Excel"],
 "acceptance_criteria":[{"id":"AC-1","given":"a report","when":"the owner exports it","then":"a CSV file downloads"}],
 "tasks":[{"id":"T-1","title":"Add the export"},{"id":"T-2","title":"Add the button"}]}`

// The words the factory adds around Lead's text follow the chat language (live check 2026-11-26: a
// Russian chat showed "I understood:", "Not in scope" and "Tasks" in English).
func TestCardLabelsFollowTheChatLanguage(t *testing.T) {
	for _, c := range []struct {
		name, chat, owner, reply string
		russian                  bool
	}{
		{"tenant fixed to ru", "ru", "Add CSV export", "All columns", true},
		{"auto follows a Russian message", "auto", "Нужна выгрузка отчёта в CSV", "Все колонки", true},
		{"auto follows an English message", "auto", "Add CSV export", "All columns", false},
		{"a tenant fixed to en", "en", "Нужна выгрузка отчёта в CSV", "Все колонки", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, rec := leadRig(t)
			r.c.ChatLanguage = c.chat
			r.say("owner_message", c.owner)
			r.advance()
			r.leadSays(`{"type":"question","understood":"CSV export.","text":"Which columns?","options":[{"label":"All"},{"label":"Visible"}],"recommended":0,"basis":{"checked":["docs/design.md section 7"],"inferred":["the columns"],"confidence":"high"}}`)
			r.advance()
			if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			want := map[bool][]string{
				true:  {"Я понял так: ", "(рекомендую)", "Проверено: ", "Предположено: ", "Уверенность: высокая"},
				false: {"I understood: ", "(recommended)", "Checked: ", "Inferred: ", "Confidence: high"},
			}[c.russian]
			wantAll(t, "question card", rec.last().Text, want)

			r.say("owner_message", c.reply)
			r.advance()
			r.leadSays(`{"type":"blocker_or_summary","text":"Spec.","spec_ready":true,"understood":"CSV export.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + russianSpec + `}`)
			r.advance()
			if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			card := rec.last()
			if card.Kind != chatapi.CardSpec {
				t.Errorf("the spec card has kind %q, want %q", card.Kind, chatapi.CardSpec)
			}
			wantAll(t, "spec card", card.Text, map[bool][]string{true: {"Вне задачи: Excel", "Задачи:", "[дополнение Lead]"}, false: {"Not in scope: Excel", "Tasks:", "[Lead's addition]"}}[c.russian])
		})
	}
}

// A plain summary keeps its own kind; only the card that waits for Approve is a spec card.
func TestASummaryIsNotASpecCard(t *testing.T) {
	r, rec := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"blocker_or_summary","text":"Not a bug.","basis":{"checked":["docs/status.md"],"confidence":"high"}}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if k := rec.last().Kind; k != chatapi.CardBlocker {
		t.Fatalf("a summary has kind %q, want %q", k, chatapi.CardBlocker)
	}
}

func wantAll(t *testing.T, what, text string, words []string) {
	t.Helper()
	for _, w := range words {
		if !strings.Contains(text, w) {
			t.Errorf("%s lacks %q: %s", what, w, text)
		}
	}
}
