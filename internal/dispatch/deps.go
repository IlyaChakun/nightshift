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
	// The row lock makes a second conductor wait, and the marker is read again under it.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM book.tasks WHERE id = $1 FOR UPDATE`, task.ID); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	qt := bookdb.New(tx)
	again, err := qt.ListTaskEvents(ctx, taskID)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if slices.ContainsFunc(again, func(e bookdb.BookEvent) bool { return e.Kind == eventSpecExpanded }) {
		return nil
	}
	ids := map[string]int64{tasks[0].ID: task.ID}
	if err := qt.SetTaskTitle(ctx, bookdb.SetTaskTitleParams{ID: task.ID, Title: tasks[0].Title}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	for _, st := range tasks[1:] {
		repo := task.Repo
		if strings.TrimSpace(st.Repo) != "" {
			repo = repoName(st.Repo)
		}
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

// checkTaskRepos refuses a spec that sends a task to a repository the tenant does not list.
func checkTaskRepos(t config.Tenant, raw json.RawMessage) error {
	tasks, err := specTasks(raw)
	if err != nil {
		return err
	}
	for _, st := range tasks {
		if strings.TrimSpace(st.Repo) == "" || len(t.Repos) == 0 { // a tenant file always lists repos; a bare one has nothing to check against
			continue
		}
		if !slices.ContainsFunc(t.Repos, func(r string) bool { return repoName(r) == repoName(st.Repo) }) {
			return fmt.Errorf("task %s names the repository %s, which the tenant does not list", st.ID, st.Repo)
		}
	}
	return nil
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
		switch {
		case flow.State(r.State) == flow.Released:
		case brokenState(r.State):
			broken = append(broken, r.ID)
			gate = depsBroken
		case gate == depsReady:
			gate = depsWaiting
		}
	}
	return gate, broken, nil
}

// blockOnDeps moves a task whose dependency was cancelled to `blocked` and tells the owner.
func (c *Conductor) blockOnDeps(ctx context.Context, t config.Tenant, task bookdb.BookTask, broken []int64) error {
	blocked, err := c.move(ctx, t, task, flow.DepCanceled, "")
	if err != nil {
		return err
	}
	list := make([]string, len(broken))
	for i, id := range broken {
		list[i] = fmt.Sprintf("#%d", id)
	}
	c.appendEvent(ctx, task, "task.blocked", "dependency cancelled: "+strings.Join(list, ", "))
	ref, err := threadOf(blocked)
	if err != nil {
		return nil //nolint:nilerr // a task without a thread (added from the CLI) has nobody to tell; the event is recorded
	}
	text := fmt.Sprintf("Task #%d is blocked: a task it waits for was cancelled (%s). Go on without it, or cancel this task?", task.ID, strings.Join(list, ", "))
	buttons := []integration.Button{{Action: "proceed", Label: "Go on without it"}, {Action: "cancel_task", Label: "Cancel this task"}}
	return c.postCard(ctx, bookdb.New(c.Pool), blocked, integration.CardBlocker, text, buttons, ref, fmt.Sprintf("dep-blocked:%d:%d", blocked.ID, blocked.Version))
}

// brokenState says whether a dependency in this state will never be released.
func brokenState(s string) bool {
	switch flow.State(s) { //nolint:exhaustive // only these three end without a release
	case flow.Canceled, flow.Rejected, flow.RolledBack:
		return true
	}
	return false
}

// dropBrokenDeps removes the cancelled dependencies of a task, after the owner chose to go on.
func dropBrokenDeps(ctx context.Context, q *bookdb.Queries, taskID int64) error {
	rows, err := q.TaskDepStates(ctx, taskID)
	if err != nil {
		return err
	}
	var ids []int64
	for _, r := range rows {
		if brokenState(r.State) {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return q.DropTaskDeps(ctx, bookdb.DropTaskDepsParams{TaskID: taskID, DependsOn: ids})
}
