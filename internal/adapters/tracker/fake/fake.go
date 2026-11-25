// Package fake is the tracker twin for tests (a Linear or Jira stand-in): it keeps the
// projected tasks and replays the two things the factory accepts back, a cancel and a new
// priority, with the same contract as a real adapter.
package fake

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// System is the adapter name the twin reports in its references.
const System = "fake-tracker"

// ErrUnknown is returned when a test changes an issue the twin never saw.
var ErrUnknown = errors.New("fake tracker: unknown issue")

// Tracker implements integration.Tracker in memory.
type Tracker struct {
	mu     sync.Mutex
	issues map[string]integration.TaskView // by issue id
	byTask map[string]string               // "tenant/task" -> issue id
	events []integration.TrackerEvent
	now    func() time.Time
}

var _ integration.Tracker = (*Tracker)(nil)

// New returns an empty twin. A nil clock uses time.Now.
func New(now func() time.Time) *Tracker {
	if now == nil {
		now = time.Now
	}
	return &Tracker{issues: map[string]integration.TaskView{}, byTask: map[string]string{}, now: now}
}

// Upsert creates the issue of a task or updates it; the same task always gets the same issue.
func (t *Tracker) Upsert(_ context.Context, v integration.TaskView) (integration.ExternalRef, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := v.Tenant + "/" + v.TaskID
	id, ok := t.byTask[key]
	if !ok {
		id = fmt.Sprintf("FAKE-%d", len(t.issues)+1)
		t.byTask[key] = id
	}
	t.issues[id] = v
	return integration.ExternalRef{System: System, ID: id, URL: "https://tracker.example.test/" + id}, nil
}

// Poll returns the changes made since the given time, oldest first.
func (t *Tracker) Poll(_ context.Context, since time.Time) ([]integration.TrackerEvent, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []integration.TrackerEvent
	for _, e := range t.events {
		if e.At.After(since) {
			out = append(out, e)
		}
	}
	return out, nil
}

// Issue is the current view of an issue, for assertions.
func (t *Tracker) Issue(id string) (integration.TaskView, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.issues[id]
	return v, ok
}

// Cancel plays a person canceling the issue in the tracker.
func (t *Tracker) Cancel(id, actor string) error {
	return t.record(id, integration.TrackerEvent{Kind: integration.TrackerCanceled, Actor: actor})
}

// SetPriority plays a person changing the issue's priority (P0..P3).
func (t *Tracker) SetPriority(id, actor, priority string) error {
	return t.record(id, integration.TrackerEvent{Kind: integration.TrackerPriorityChanged, Actor: actor, Priority: priority})
}

func (t *Tracker) record(id string, e integration.TrackerEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.issues[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknown, id)
	}
	e.Ref = integration.ExternalRef{System: System, ID: id, URL: "https://tracker.example.test/" + id}
	e.At = t.now()
	t.events = append(t.events, e)
	return nil
}
