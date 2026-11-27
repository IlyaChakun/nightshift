package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
)

// kindHidden is the document kind of a task's hidden acceptance scenarios. They are stored with
// visibility `lead_verifier`, so a search made for Builder never returns them (design section 13).
const kindHidden = "hidden_scenarios"

// storeHidden keeps the hidden scenarios of an approved-to-be spec in the book. A newer set for
// the same task replaces the older one, which stays as `superseded`. The scenarios are never part
// of the spec event, the card or the repository.
func (c *Conductor) storeHidden(ctx context.Context, task bookdb.BookTask, raw json.RawMessage) error {
	var r struct {
		Scenarios json.RawMessage `json:"hidden_scenarios"`
	}
	source := "task:" + strconv.FormatInt(task.ID, 10)
	// A new spec replaces the old scenarios even when it brings none, so the Verifier never
	// checks an earlier spec's scenarios against a later spec.
	if _, err := c.Pool.Exec(ctx, `UPDATE book.documents SET status = 'superseded' WHERE tenant = $1 AND kind = $2 AND source_url = $3 AND status <> 'superseded'`, task.Tenant, kindHidden, source); err != nil {
		return fmt.Errorf("conductor: hidden scenarios: %w", err)
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("conductor: hidden scenarios: %w", err)
	}
	if len(r.Scenarios) == 0 || string(r.Scenarios) == "null" || string(r.Scenarios) == "[]" {
		return nil
	}
	if _, err := (&search.Index{Pool: c.Pool}).Add(ctx, search.Document{
		Tenant: task.Tenant, Kind: kindHidden, Source: source, Title: "Hidden scenarios of task " + strconv.FormatInt(task.ID, 10),
		Body: string(r.Scenarios), Visibility: "lead_verifier",
	}); err != nil {
		return fmt.Errorf("conductor: hidden scenarios: %w", err)
	}
	return nil
}

// HiddenScenarios returns the current hidden scenarios of a task as JSON, or nil when it has
// none. Only the Verifier's acceptance job may be given them (jobsapi.VisibleInputs drops the
// `hidden_scenarios` input for every other role).
func HiddenScenarios(ctx context.Context, ix *search.Index, tenant string, taskID int64) (json.RawMessage, error) {
	var body string
	err := ix.Pool.QueryRow(ctx, `SELECT body FROM book.documents WHERE tenant = $1 AND kind = $2 AND source_url = $3 AND status = 'active' ORDER BY id DESC LIMIT 1`,
		tenant, kindHidden, "task:"+strconv.FormatInt(taskID, 10)).Scan(&body)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("conductor: hidden scenarios: %w", err)
	}
	return json.RawMessage(body), nil
}
