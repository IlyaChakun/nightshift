package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// eventHandoff stores a role's latest handoff; the actor is the role (design section 12).
const eventHandoff = "handoff"

// maxHandoffBytes is the size limit of a stored handoff.
const maxHandoffBytes = 2048

// minimalEvents is how many recent events a handoff built without a model lists.
const minimalEvents = 8

// storeHandoff keeps the handoff block of a role's result as an event. A missing or oversized
// block is dropped: the handoff is a hint and must never fail a task.
func (c *Conductor) storeHandoff(ctx context.Context, task bookdb.BookTask, role string, raw json.RawMessage) {
	if len(raw) == 0 || len(raw) > maxHandoffBytes || string(raw) == "null" {
		return
	}
	if _, err := bookdb.New(c.Pool).AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventHandoff, Actor: role, Payload: raw,
	}); err != nil {
		c.log().WarnContext(ctx, "handoff not recorded", "task", task.ID, "role", role, "error", err)
	}
}

// handoffOf is the text a fresh session of the role gets under "Previous session, unverified":
// the latest stored handoff, or a minimal one from the task's events when none was written.
// It is empty for a task with nothing to hand over.
func (c *Conductor) handoffOf(ctx context.Context, task bookdb.BookTask, role string) string {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return ""
	}
	return handoffText(events, role)
}

func handoffText(events []bookdb.BookEvent, role string) string {
	if h := storedHandoff(events, role); h != "" {
		return h
	}
	return minimalHandoff(events)
}

// storedHandoff is the latest handoff a role wrote, or empty.
func storedHandoff(events []bookdb.BookEvent, role string) string {
	for _, e := range slices.Backward(events) {
		if e.Kind != eventHandoff || e.Actor != role {
			continue
		}
		var h struct {
			Done      []string `json:"done"`
			Pending   []string `json:"pending"`
			Decisions []string `json:"decisions"`
		}
		if json.Unmarshal(e.Payload, &h) != nil {
			break
		}
		var b strings.Builder
		for _, s := range []struct {
			name  string
			items []string
		}{{"Done", h.Done}, {"Pending", h.Pending}, {"Decisions", h.Decisions}} {
			if len(s.items) > 0 {
				fmt.Fprintf(&b, "%s:\n- %s\n", s.name, strings.Join(s.items, "\n- "))
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// minimalHandoff lists the latest events of the task, with no model: what the factory itself
// recorded is all a restarted session can rely on.
func minimalHandoff(events []bookdb.BookEvent) string {
	var kinds []string
	for _, e := range events {
		if e.Kind != eventHandoff && e.Kind != "owner_message" && e.Kind != "task_created" {
			kinds = append(kinds, e.Kind)
		}
	}
	if len(kinds) == 0 {
		return ""
	}
	if len(kinds) > minimalEvents {
		kinds = kinds[len(kinds)-minimalEvents:]
	}
	return "No handoff was written. The task's latest events, oldest first:\n- " + strings.Join(kinds, "\n- ")
}
