package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// eventSpecExpanded marks a task whose spec has been turned into tasks and links.
const eventSpecExpanded = "spec.expanded"

// eventSpecScheduled marks an applied schedule without changing single-task spec semantics.
const eventSpecScheduled = "spec.scheduled"

// specTask is one task of an approved spec (schemas/spec.json).
type specTask struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Repo      string     `json:"repo"`
	NotBefore *specStart `json:"not_before"`
	Priority  *float64   `json:"priority"`
	DependsOn []string   `json:"depends_on"`
}

type specStart time.Time

func (at *specStart) UnmarshalText(raw []byte) error {
	parsed, err := time.Parse(time.RFC3339, strings.ToUpper(string(raw)))
	*at = specStart(parsed)
	return err
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
	return findCycle(tasks, byID)
}

// findCycle reports a dependency cycle by depth-first search.
func findCycle(tasks []specTask, byID map[string]specTask) error {
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

// approvedTasks finds the newest approved spec that still needs task expansion.
func approvedTasks(ctx context.Context, q *bookdb.Queries, id int64) ([]specTask, error) {
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: id, Valid: true})
	if err != nil {
		return nil, err
	}
	for _, e := range slices.Backward(events) {
		switch e.Kind {
		case eventSpecExpanded, eventSpecScheduled:
			return nil, nil
		case eventLeadSpec:
			tasks, err := specTasks(e.Payload)
			if err != nil {
				return nil, err
			}
			if len(tasks) == 1 && !tasks[0].scheduled() {
				return nil, nil
			}
			return tasks, checkTaskGraph(e.Payload)
		}
	}
	return nil, nil
}

func (s specTask) scheduled() bool { return s.NotBefore != nil || s.Priority != nil }

func (s specTask) schedule(priority int32, at pgtype.Timestamptz) (int32, pgtype.Timestamptz) {
	if s.Priority != nil {
		priority = int32(*s.Priority)
	}
	if s.NotBefore != nil {
		at = pgtype.Timestamptz{Time: time.Time(*s.NotBefore), Valid: true}
	}
	return priority, at
}

// expandSpec handles tasks queued by an earlier release. New approvals expand in the transaction
// that moves the parent to queued, before a scheduler can see it.
func (c *Conductor) expandSpec(ctx context.Context, task bookdb.BookTask) (bool, error) {
	tasks, err := approvedTasks(ctx, bookdb.New(c.Pool), task.ID)
	if err != nil || len(tasks) == 0 {
		return false, err
	}
	err = pgx.BeginFunc(ctx, c.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		locked, err := q.LockTask(ctx, task.ID)
		if err != nil {
			return err
		}
		_, err = ExpandApprovedTasks(ctx, q, locked)
		return err
	})
	return slices.ContainsFunc(tasks, specTask.scheduled), err
}

// ExpandApprovedTasks runs with the parent row locked. The marker and all siblings commit together.
func ExpandApprovedTasks(ctx context.Context, qt *bookdb.Queries, task bookdb.BookTask) (bool, error) {
	tasks, err := approvedTasks(ctx, qt, task.ID)
	if err != nil || len(tasks) == 0 {
		return false, err
	}
	taskID := pgtype.Int8{Int64: task.ID, Valid: true}

	ids := map[string]int64{tasks[0].ID: task.ID}
	priority, at := tasks[0].schedule(task.Priority, task.NotBefore)
	title := task.Title
	if len(tasks) > 1 {
		title = tasks[0].Title
	}
	if err := qt.SetSpecTask(ctx, bookdb.SetSpecTaskParams{ID: task.ID, Title: title, Priority: priority, NotBefore: at}); err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	for _, st := range tasks[1:] {
		repo := task.Repo
		if strings.TrimSpace(st.Repo) != "" {
			repo = config.RepoName(st.Repo)
		}
		priority, at := st.schedule(task.Priority, pgtype.Timestamptz{})
		nt, err := qt.CreateSpecTask(ctx, bookdb.CreateSpecTaskParams{
			Tenant: task.Tenant, Repo: repo, Title: st.Title, ThreadRef: task.ThreadRef,
			Priority: priority, NotBefore: at, AllowFallback: task.AllowFallback,
		})
		if err != nil {
			return false, fmt.Errorf("conductor: %w", err)
		}
		ids[st.ID] = nt.ID
	}
	for _, st := range tasks {
		for _, d := range st.DependsOn {
			if err := qt.AddTaskDep(ctx, bookdb.AddTaskDepParams{TaskID: ids[st.ID], DependsOn: ids[d]}); err != nil {
				return false, fmt.Errorf("conductor: %w", err)
			}
		}
	}
	kind := eventSpecExpanded
	if len(tasks) == 1 {
		kind = eventSpecScheduled
	}
	// The map of spec task ids to book tasks is the only record of which tasks one spec made,
	// siblings without a dependency edge included (plan item A05b proposes a check that reads it).
	payload, err := json.Marshal(map[string]map[string]int64{"tasks": ids})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	if _, err := qt.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: taskID, Kind: kind, Actor: "system", Payload: payload,
	}); err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	return slices.ContainsFunc(tasks, specTask.scheduled), nil
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
		if !slices.ContainsFunc(t.Repos, func(r string) bool { return config.RepoName(r) == config.RepoName(st.Repo) }) {
			return fmt.Errorf("task %s names the repository %s, which the tenant does not list", st.ID, st.Repo)
		}
	}
	return nil
}

// closedDep says whether a dependency is done: released, or merged by someone else (the team
// profile, or a pull request merged outside the factory).
func closedDep(s string) bool {
	return flow.State(s) == flow.Released || flow.State(s) == flow.MergedByTeam
}

// depsGate says what a queued task waits for: ready when every dependency is closed, a
// canceled or failed dependency blocks it, anything else is waiting.
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
		case closedDep(r.State):
		case brokenState(r.State):
			broken = append(broken, r.ID)
			gate = depsBroken
		case gate == depsReady:
			gate = depsWaiting
		}
	}
	return gate, broken, nil
}

// blockOnDeps moves a task whose dependency was canceled to `blocked` and tells the owner.
func (c *Conductor) blockOnDeps(ctx context.Context, t config.Tenant, task bookdb.BookTask, broken []int64) error {
	blocked, err := c.move(ctx, t, task, flow.DepCanceled, "")
	if err != nil {
		return err
	}
	list := make([]string, len(broken))
	for i, id := range broken {
		list[i] = fmt.Sprintf("#%d", id)
	}
	c.appendEvent(ctx, task, "task.blocked", "dependency canceled: "+strings.Join(list, ", "))
	ref, err := ThreadOf(blocked)
	if err != nil {
		return nil //nolint:nilerr // a task without a thread (added from the CLI) has nobody to tell; the event is recorded
	}
	text := fmt.Sprintf("Task #%d is blocked: a task it waits for was canceled (%s). Go on without it, or cancel this task?", task.ID, strings.Join(list, ", "))
	buttons := []chatapi.Button{{Action: "proceed", Label: "Go on without it"}, {Action: "cancel_task", Label: "Cancel this task"}}
	return c.postCard(ctx, bookdb.New(c.Pool), blocked, chatapi.CardBlocker, text, buttons, ref, fmt.Sprintf("dep-blocked:%d:%d", blocked.ID, blocked.Version))
}

// brokenState says whether a dependency in this state will never be released.
func brokenState(s string) bool {
	switch flow.State(s) { //nolint:exhaustive // only these three end without a release
	case flow.Canceled, flow.Rejected, flow.RolledBack:
		return true
	}
	return false
}

// DropBrokenDeps removes the canceled dependencies of a task, after the owner chose to go on.
func DropBrokenDeps(ctx context.Context, q *bookdb.Queries, taskID int64) error {
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
