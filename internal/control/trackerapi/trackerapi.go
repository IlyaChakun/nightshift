// Package trackerapi is the issue tracker as the conductor uses it (design section 21): it
// projects tasks into the tracker and reads back cancel and priority. Adapters live in
// internal/adapters and are chosen by name from tenant.yaml.
package trackerapi

import (
	"context"
	"time"
)

// Tracker projects tasks into an issue tracker and reads back the only two
// things the factory accepts from it: cancel and priority.
type Tracker interface {
	Upsert(ctx context.Context, t TaskView) (ExternalRef, error)
	Poll(ctx context.Context, since time.Time) ([]TrackerEvent, error)
}

// ExternalRef points to an object in an external system.
type ExternalRef struct {
	System string `json:"system"` // adapter name from tenant.yaml
	ID     string `json:"id"`
	URL    string `json:"url"`
}

// TaskView is what the tracker shows about a task.
type TaskView struct {
	Tenant    string `json:"tenant"`
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Priority  string `json:"priority"` // P0..P3
	ThreadURL string `json:"thread_url,omitempty"`
}

// TrackerEventKind is what the factory accepts from a tracker.
type TrackerEventKind string

// Tracker event kinds.
const (
	TrackerCanceled        TrackerEventKind = "canceled"
	TrackerPriorityChanged TrackerEventKind = "priority_changed"
)

// TrackerEvent is a change made in the tracker. An adapter returns the events after `since` in
// time order, and names the actor by an id it has verified (the tenant's chat or GitHub id), not
// by a display name.
type TrackerEvent struct {
	Ref      ExternalRef      `json:"ref"`
	Kind     TrackerEventKind `json:"kind"`
	Priority string           `json:"priority,omitempty"`
	Actor    string           `json:"actor"` // checked against owner and approvers
	At       time.Time        `json:"at"`
}
