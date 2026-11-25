package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// eventSpecExpanded marks a task whose spec has been turned into tasks and links.
const eventSpecExpanded = "spec.expanded"

// specTask is one task of an approved spec (schemas/spec.json).
type specTask struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Repo      string   `json:"repo"`
	DependsOn []string `json:"depends_on"`
}

func specTasks(raw json.RawMessage) ([]specTask, error) {
	var s struct {
		Tasks []specTask `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("the spec is not JSON: %w", err)
	}
	return s.Tasks, nil
}

// checkTaskGraph refuses a spec whose tasks repeat an id, depend on an unknown task or on
// themselves, or form a cycle. Lead gets the reason and writes the spec again.
func checkTaskGraph(raw json.RawMessage) error {
	tasks, err := specTasks(raw)
	if err != nil {
		return err
	}
	byID := map[string]specTask{}
	for _, t := range tasks {
		if _, dup := byID[t.ID]; dup {
			return fmt.Errorf("the spec repeats the task id %s", t.ID)
		}
		byID[t.ID] = t
	}
	for _, t := range tasks {
		for _, d := range t.DependsOn {
			if d == t.ID {
				return fmt.Errorf("task %s depends on itself", t.ID)
			}
			if _, ok := byID[d]; !ok {
				return fmt.Errorf("task %s depends on %s, which the spec does not have", t.ID, d)
			}
		}
	}
	state := map[string]int{} // 1 on the path, 2 done
	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("the task dependencies form a cycle through %s", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, d := range byID[id].DependsOn {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, t := range tasks {
		if err := visit(t.ID); err != nil {
			return err
		}
	}
	return nil
}

// expandSpec turns the tasks of the approved spec into tasks of the book, once. The task the
// owner approved is the spec's first task; the others are created queued, with their
// dependency links. A spec with one task changes nothing.
func (c *Conductor) expandSpec(ctx context.Context, task bookdb.BookTask) error {
	q := bookdb.New(c.Pool)
	taskID := pgtype.Int8{Int64: task.ID, Valid: true}
	events, err := q.ListTaskEvents(ctx, taskID)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	var raw json.RawMessage
	for _, e := range slices.Backward(events) {
		if e.Kind == eventSpecExpanded {
			return nil
		}
		if e.Kind == eventLeadSpec && raw == nil {
			raw = e.Payload
		}
	}
	if raw == nil {
		return nil
	}
	tasks, err := specTasks(raw)
	if err != nil || len(tasks) < 2 {
		return nil //nolint:nilerr // a spec that cannot be expanded runs as one task; Lead's reply check refuses a bad graph before approval
	}
	if err := checkTaskGraph(raw); err != nil {
		return fmt.Errorf("conductor: task %d: %w", task.ID, err)
	}
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qt := bookdb.New(tx)
	ids := map[string]int64{tasks[0].ID: task.ID}
	if err := qt.SetTaskTitle(ctx, bookdb.SetTaskTitleParams{ID: task.ID, Title: tasks[0].Title}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	for _, st := range tasks[1:] {
		repo := cmpOr(st.Repo, task.Repo)
		nt, err := qt.CreateSpecTask(ctx, bookdb.CreateSpecTaskParams{
			Tenant: task.Tenant, Repo: repo, Title: st.Title, ThreadRef: task.ThreadRef,
			Priority: task.Priority, AllowFallback: task.AllowFallback,
		})
		if err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
		ids[st.ID] = nt.ID
	}
	for _, st := range tasks {
		for _, d := range st.DependsOn {
			if err := qt.AddTaskDep(ctx, bookdb.AddTaskDepParams{TaskID: ids[st.ID], DependsOn: ids[d]}); err != nil {
				return fmt.Errorf("conductor: %w", err)
			}
		}
	}
	if _, err := qt.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: taskID, Kind: eventSpecExpanded, Actor: "system", Payload: []byte(`{}`),
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	return tx.Commit(ctx)
}

func cmpOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// depsGate says what a queued task waits for: ready when every dependency is released, a
// cancelled or failed dependency blocks it, anything else is waiting.
type depsGate int

const (
	depsReady depsGate = iota
	depsWaiting
	depsBroken
)

func (c *Conductor) depsGate(ctx context.Context, task bookdb.BookTask) (depsGate, []int64, error) {
	rows, err := bookdb.New(c.Pool).TaskDepStates(ctx, task.ID)
	if err != nil {
		return depsWaiting, nil, fmt.Errorf("conductor: %w", err)
	}
	gate := depsReady
	var broken []int64
	for _, r := range rows {
		switch flow.State(r.State) { //nolint:exhaustive // every other state means the dependency is still on its way
		case flow.Released:
		case flow.Canceled, flow.Rejected, flow.RolledBack:
			broken = append(broken, r.ID)
			gate = depsBroken
		default:
			if gate == depsReady {
				gate = depsWaiting
			}
		}
	}
	return gate, broken, nil
}

// blockOnDeps moves a task whose dependency was cancelled to `blocked` and tells the owner.
func (c *Conductor) blockOnDeps(ctx context.Context, t config.Tenant, task bookdb.BookTask, broken []int64) error {
	if _, err := c.move(ctx, t, task, flow.DepCanceled, ""); err != nil {
		return err
	}
	list := make([]string, len(broken))
	for i, id := range broken {
		list[i] = fmt.Sprintf("#%d", id)
	}
	c.appendEvent(ctx, task, "task.blocked", "dependency cancelled: "+strings.Join(list, ", "))
	ref, err := threadOf(task)
	if err != nil {
		return nil //nolint:nilerr // a task without a thread (added from the CLI) has nobody to tell; the event is recorded
	}
	text := fmt.Sprintf("Task #%d is blocked: a task it waits for was cancelled (%s). Decide whether to go on without it or cancel this task.", task.ID, strings.Join(list, ", "))
	return c.postCard(ctx, bookdb.New(c.Pool), task, integration.CardBlocker, text, nil, ref, fmt.Sprintf("dep-blocked:%d:%d", task.ID, task.Version))
}
