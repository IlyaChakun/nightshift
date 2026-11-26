package dispatch_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/integration"
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
			q := rec.last()
			want := map[bool][]string{
				true:  {"Я понял так: ", "(рекомендую)", "Проверено: ", "Предположено: ", "Уверенность: высокая"},
				false: {"I understood: ", "(recommended)", "Checked: ", "Inferred: ", "Confidence: high"},
			}[c.russian]
			for _, w := range want {
				if !strings.Contains(q.Text, w) {
					t.Errorf("question card lacks %q: %s", w, q.Text)
				}
			}

			r.say("owner_message", c.reply)
			r.advance()
			r.leadSays(`{"type":"blocker_or_summary","text":"Spec.","spec_ready":true,"understood":"CSV export.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + russianSpec + `}`)
			r.advance()
			if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			card := rec.last()
			if card.Kind != integration.CardSpec {
				t.Errorf("the spec card has kind %q, want %q", card.Kind, integration.CardSpec)
			}
			wantSpec := map[bool][]string{true: {"Вне задачи: Excel", "Задачи:", "[дополнение Lead]"}, false: {"Not in scope: Excel", "Tasks:", "[Lead's addition]"}}[c.russian]
			for _, w := range wantSpec {
				if !strings.Contains(card.Text, w) {
					t.Errorf("spec card lacks %q: %s", w, card.Text)
				}
			}
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
	if k := rec.last().Kind; k != integration.CardBlocker {
		t.Fatalf("a summary has kind %q, want %q", k, integration.CardBlocker)
	}
}
