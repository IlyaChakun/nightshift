package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// checkSpecQuotes reads the task's owner messages and checks the spec's quotes against them.
func (c *Conductor) checkSpecQuotes(ctx context.Context, task bookdb.BookTask, spec json.RawMessage) error {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	return checkQuotes(events, spec)
}

// checkQuotes refuses a spec whose `source_quote` is not in an owner message of the task, so a
// criterion is tied to the owner's own words or is left unmarked and shows as Lead's addition
// (docs/research/task-intake.md, gap 2).
func checkQuotes(events []bookdb.BookEvent, spec json.RawMessage) error {
	var s struct {
		Criteria []struct {
			ID    string `json:"id"`
			Quote string `json:"source_quote"`
		} `json:"acceptance_criteria"`
	}
	if json.Unmarshal(spec, &s) != nil {
		return nil //nolint:nilerr // the spec was validated against its schema before this
	}
	var owner []string
	for _, e := range events {
		if e.Kind != "task_created" && e.Kind != "owner_message" {
			continue
		}
		var m struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &m) == nil {
			owner = append(owner, normalize(m.Text))
		}
	}
	for _, c := range s.Criteria {
		quote := normalize(c.Quote)
		if quote == "" {
			continue
		}
		found := false
		for _, m := range owner {
			if strings.Contains(m, quote) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: source_quote %q is in no owner message of this task; copy the owner's words exactly or leave the quote out", c.ID, c.Quote)
		}
	}
	return nil
}
