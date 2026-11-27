// Package tracking keeps a tenant's tracker in step with the book and reads back the two
// changes the factory accepts from it: a cancel and a priority (design section 7).
package tracking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/control/trackerapi"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// TrackerTarget is the outbox target of tracker projections.
const TrackerTarget = "tracker"

// eventTrackerProjected records the issue a task was projected to: the book keeps the link, so a
// change made in the tracker finds its task.
const eventTrackerProjected = "tracker.projected"

// A task starts projecting once it reaches the queue. The move history keeps
// projections working after cancellation, escalation or a return to Lead.
const trackerApproved = `(t.state = 'queued' OR EXISTS (
    SELECT 1 FROM book.events e WHERE e.task_id = t.id AND e.kind = 'task.moved'
    AND (e.payload->>'from' = 'queued' OR e.payload->>'to' = 'queued')))`

// recentWindow is how far back a finished task is still projected, so that its last state
// reaches the tracker.
const recentWindow = "24 hours"

// trackerBackfill is how far before the start the first poll looks. A cancel or a priority
// change repeats safely, so the window may overlap a restart.
const trackerBackfill = time.Hour

// Priorities of the tracker (P0 is the most urgent) as the book's integer priority.
var trackerPriorities = map[string]int32{"P0": 10, "P1": 5, "P2": 0, "P3": -5}

func priorityName(p int32) string {
	switch {
	case p >= 10:
		return "P0"
	case p >= 5:
		return "P1"
	case p >= 0:
		return "P2"
	}
	return "P3"
}

// TrackerSender delivers projections: one tracker per tenant.
type TrackerSender struct {
	Pool     *pgxpool.Pool
	Trackers map[string]trackerapi.Tracker
}

// Send projects one task view. The same task always gets the same issue (the adapter's contract),
// so a redelivery updates it again.
func (s TrackerSender) Send(ctx context.Context, m outbox.Message) error {
	tr, ok := s.Trackers[m.Tenant]
	if !ok {
		return fmt.Errorf("%w: tenant %s has no tracker", outbox.ErrPermanent, m.Tenant)
	}
	// The message says that a task changed; what goes out is the task as it is now, so a retried
	// older message can never put an old state over a newer one.
	var v trackerapi.TaskView
	var priority int32
	var approved bool
	if err := s.Pool.QueryRow(ctx, `SELECT title, state, priority, `+trackerApproved+` FROM book.tasks t
		WHERE id = $1 AND tenant = $2`, m.TaskID, m.Tenant).Scan(&v.Title, &v.State, &priority, &approved); err != nil {
		return fmt.Errorf("%w: task %d: %w", outbox.ErrPermanent, m.TaskID, err)
	}
	// An older release may already have queued a projection for a draft.
	if !approved {
		return nil
	}
	v.Tenant, v.TaskID, v.Priority = m.Tenant, strconv.FormatInt(m.TaskID, 10), priorityName(priority)
	ref, err := tr.Upsert(ctx, v)
	if err != nil {
		return fmt.Errorf("tracker: upsert: %w", err)
	}
	payload, err := json.Marshal(ref)
	if err != nil {
		return fmt.Errorf("tracker: %w", err)
	}
	_, err = bookdb.New(s.Pool).AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: m.Tenant, TaskID: pgtype.Int8{Int64: m.TaskID, Valid: m.TaskID != 0}, Kind: eventTrackerProjected, Actor: "system", Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("tracker: record: %w", err)
	}
	return nil
}

// Projector keeps a tracker's view of the tenant's tasks and reads back the only two changes the
// factory accepts: a cancel and a priority (design section 7). The book stays the source of truth.
type Projector struct {
	Pool   *pgxpool.Pool
	Outbox *outbox.Outbox
	Mover  *flow.Mover
	// Tracker returns the tenant's tracker, or nil when it has none.
	Tracker func(config.Tenant) trackerapi.Tracker
	Log     *slog.Logger

	mu    sync.Mutex
	since map[string]time.Time // tenant -> the newest tracker event handled
}

// Run projects the tasks that changed and applies what people changed in the tracker.
func (p *Projector) Run(ctx context.Context, t config.Tenant) {
	tr := p.Tracker(t)
	if tr == nil {
		return
	}
	if err := p.project(ctx, t); err != nil {
		p.log().WarnContext(ctx, "tracker projection failed", "tenant", t.Name, "error", err)
	}
	if err := p.poll(ctx, t, tr); err != nil {
		p.log().WarnContext(ctx, "tracker poll failed", "tenant", t.Name, "error", err)
	}
}

func (p *Projector) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.Default()
}

// project queues one message per task state; a state already queued is a no-op (its key).
func (p *Projector) project(ctx context.Context, t config.Tenant) error {
	rows, err := p.Pool.Query(ctx, `SELECT id, title, state, version, priority FROM book.tasks t
		WHERE tenant = $1 AND (state = ANY($2) OR updated_at > now() - $3::interval)
		AND `+trackerApproved+` ORDER BY id`,
		t.Name, append(dispatch.ActiveStates(), string(flow.Spec)), recentWindow)
	if err != nil {
		return fmt.Errorf("tracker: list tasks: %w", err)
	}
	type row struct {
		id       int64
		title    string
		state    string
		version  int32
		priority int32
	}
	var tasks []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.title, &r.state, &r.version, &r.priority); err != nil {
			rows.Close()
			return fmt.Errorf("tracker: list tasks: %w", err)
		}
		tasks = append(tasks, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("tracker: list tasks: %w", err)
	}
	q := bookdb.New(p.Pool)
	for _, r := range tasks {
		view, err := json.Marshal(trackerapi.TaskView{
			Tenant: t.Name, TaskID: strconv.FormatInt(r.id, 10), Title: r.title, State: r.state, Priority: priorityName(r.priority),
		})
		if err != nil {
			return fmt.Errorf("tracker: %w", err)
		}
		// A state and a priority each make a new message.
		key := "tracker:" + strconv.FormatInt(r.id, 10) + ":" + strconv.Itoa(int(r.version)) + ":" + strconv.Itoa(int(r.priority))
		if err := p.Outbox.Enqueue(ctx, q, t.Name, r.id, TrackerTarget, key, view); err != nil {
			return err
		}
	}
	return nil
}

// poll applies a cancel or a priority change that a tenant person made in the tracker. The adapter
// names the actor by a chat or GitHub id of the tenant; anyone but the owner and the approvers is
// ignored.
func (p *Projector) poll(ctx context.Context, t config.Tenant, tr trackerapi.Tracker) error {
	p.mu.Lock()
	if p.since == nil {
		p.since = map[string]time.Time{}
	}
	since, ok := p.since[t.Name]
	if !ok {
		since = time.Now().Add(-trackerBackfill)
	}
	p.mu.Unlock()
	events, err := tr.Poll(ctx, since)
	if err != nil {
		return fmt.Errorf("tracker: poll: %w", err)
	}
	newest := since
	for _, e := range events {
		if e.At.After(newest) {
			newest = e.At
		}
		if !mayDecide(t, e.Actor) {
			p.log().WarnContext(ctx, "tracker change by someone who may not decide", "tenant", t.Name, "kind", e.Kind)
			continue
		}
		taskID, err := p.taskOf(ctx, t.Name, e.Ref)
		if err != nil {
			p.log().WarnContext(ctx, "tracker change for an unknown issue", "tenant", t.Name, "issue", e.Ref.ID)
			continue
		}
		switch e.Kind {
		case trackerapi.TrackerCanceled:
			err = p.cancel(ctx, t, taskID, e.Actor)
		case trackerapi.TrackerPriorityChanged:
			prio, known := trackerPriorities[e.Priority]
			if !known {
				continue
			}
			_, err = p.Pool.Exec(ctx, `UPDATE book.tasks SET priority = $2 WHERE id = $1 AND tenant = $3`, taskID, prio, t.Name)
		}
		if err != nil {
			return fmt.Errorf("tracker: apply %s: %w", e.Kind, err)
		}
	}
	p.mu.Lock()
	p.since[t.Name] = newest
	p.mu.Unlock()
	return nil
}

// mayDecide says whether an actor id is the tenant's owner or an approver.
func mayDecide(t config.Tenant, actor string) bool {
	if actor == "" {
		return false
	}
	return actor == t.People.Owner.Chat || actor == t.People.Owner.GitHub || slices.Contains(t.People.Approvers, actor)
}

// taskOf finds the task that was projected to an issue.
func (p *Projector) taskOf(ctx context.Context, tenant string, ref trackerapi.ExternalRef) (int64, error) {
	var id int64
	err := p.Pool.QueryRow(ctx, `SELECT task_id FROM book.events WHERE tenant = $1 AND kind = $2
		AND payload->>'system' = $3 AND payload->>'id' = $4 ORDER BY id DESC LIMIT 1`,
		tenant, eventTrackerProjected, ref.System, ref.ID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("tracker: no task for issue %s: %w", ref.ID, err)
	}
	return id, nil
}

// cancel moves the task to canceled. Losing the race to the conductor (flow.ErrConflict) is
// retried, because Apply reads the task again; a still-lost cancel is an error, so the poll
// cursor stays and the next tick sees the event again. A task that is already over needs no
// cancel.
func (p *Projector) cancel(ctx context.Context, t config.Tenant, taskID int64, actor string) error {
	var err error
	for range 3 {
		_, err = p.Mover.Apply(ctx, flow.Move{TaskID: taskID, Event: flow.Cancel, Actor: actor, Profile: flow.ProfileFor(t.Governance)})
		if !errors.Is(err, flow.ErrConflict) {
			break
		}
	}
	if errors.Is(err, flow.ErrNotAllowed) {
		return nil
	}
	return err
}
