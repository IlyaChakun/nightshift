package dispatch

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// cardLabels are the fixed words the factory adds around Lead's text on a card. Lead writes the
// body in the chat language (`languageNote`); these labels are code, so they follow the same
// language: Russian when the chat is Russian, English otherwise (the only other set).
type cardLabels struct {
	Understood, Checked, Inferred, Confidence string
	NotInScope, Tasks, DuplicateOf, Thread    string
	Recommended, LeadAddition                 string
	Level                                     map[string]string // confidence values
}

func labelsFor(ru bool) cardLabels {
	if ru {
		return cardLabels{
			Understood: "Я понял так", Checked: "Проверено", Inferred: "Предположено", Confidence: "Уверенность",
			NotInScope: "Вне задачи", Tasks: "Задачи", DuplicateOf: "Дубликат задачи", Thread: "тред",
			Recommended: "рекомендую", LeadAddition: "дополнение Lead",
			Level: map[string]string{"high": "высокая", "medium": "средняя", "low": "низкая"},
		}
	}
	return cardLabels{
		Understood: "I understood", Checked: "Checked", Inferred: "Inferred", Confidence: "Confidence",
		NotInScope: "Not in scope", Tasks: "Tasks", DuplicateOf: "Duplicate of task", Thread: "thread",
		Recommended: "recommended", LeadAddition: "Lead's addition",
	}
}

func (l cardLabels) level(v string) string {
	if t, ok := l.Level[v]; ok {
		return t
	}
	return v
}

// labelsOf picks the labels for a task's cards from the tenant's chat language, or, for `auto`,
// from the owner's last message.
func (c *Conductor) labelsOf(ctx context.Context, task bookdb.BookTask) cardLabels {
	if c.ChatLanguage != "" && c.ChatLanguage != "auto" {
		return labelsFor(wantsRussian(c.ChatLanguage, ""))
	}
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return labelsFor(false)
	}
	texts := ownerTexts(events)
	if len(texts) == 0 {
		return labelsFor(false)
	}
	return labelsFor(isRussian(texts[len(texts)-1]))
}
