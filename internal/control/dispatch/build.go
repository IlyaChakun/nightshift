package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/assurance/standards"
	"github.com/ilyachakun/nightshift/internal/control/codehost"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/routing"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// building runs the Builder leg: enqueue a job, wait for it, then either send the findings
// back or turn the patch into a pushed branch and a draft pull request.
func (c *Conductor) building(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	q := bookdb.New(c.Pool)
	jobs, err := q.ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: task.ID, Role: "builder"})
	if err != nil {
		return fmt.Errorf("conductor: jobs of task %d: %w", task.ID, err)
	}
	if len(jobs) == 0 || jobs[len(jobs)-1].ConsumedAt.Valid {
		return c.enqueueBuilder(ctx, task)
	}
	last := jobs[len(jobs)-1]
	switch last.State {
	case "queued", "running":
		return nil
	case "done":
		return c.consume(ctx, t, task, last)
	default: // failed or canceled: the worker could not finish it
		if limitHit(last) {
			return c.requeueAfterLimit(ctx, task, last)
		}
		return c.jobFailed(ctx, t, task, last, "the Builder job ended with "+last.ExitReason.String)
	}
}

// escalate ends a job and sends the task to `escalated` with e for the owner to decide.
func (c *Conductor) escalate(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, e flow.Event, why string) error {
	if _, err := bookdb.New(c.Pool).ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	return c.escalateTask(ctx, t, task, e, why)
}

// jobFailed ends a job that stopped without a result and escalates the task. A job stopped by
// its budget (exit reason "budget": hours, turns, tokens or cost) escalates as budget_exhausted;
// any other end (an error, a stopped worker) as job_failed, so the owner does not read it as a
// spent budget.
func (c *Conductor) jobFailed(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, why string) error {
	e := flow.JobFailed
	if job.ExitReason.String == exitBudget {
		e = flow.BudgetExhausted
	}
	return c.escalate(ctx, t, task, job, e, why)
}

// exitBudget is the exit reason of a job its budget stopped (jobsapi, harness.ExitBudget).
const exitBudget = "budget"

// consume acts on the result of a finished Builder job.
func (c *Conductor) consume(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob) error {
	q := bookdb.New(c.Pool)
	var res struct {
		Status        string          `json:"status"`
		Summary       string          `json:"summary"`
		BlockedReason string          `json:"blocked_reason"`
		Handoff       json.RawMessage `json:"handoff"`
	}
	if err := json.Unmarshal(job.Result, &res); err != nil {
		return c.escalate(ctx, t, task, job, flow.Escalate, "the Builder result is not JSON")
	}
	if res.Status == "blocked" {
		c.storeHandoff(ctx, task, job.ID, "builder", res.Handoff)
		if back, err := c.backToLead(ctx, t, task, job, res.BlockedReason); back || err != nil {
			return err
		}
		return c.escalate(ctx, t, task, job, flow.Escalate, "Builder is blocked: "+res.BlockedReason)
	}
	found, err := q.ListJobFindings(ctx, pgtype.Int8{Int64: job.ID, Valid: true})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	blocking := false
	for _, f := range found {
		blocking = blocking || (f.Severity == "block" && f.Status == "open")
	}
	if blocking {
		return c.sendBack(ctx, t, task, job, res.Handoff, "the checks found problems")
	}
	patch, err := c.patchOf(ctx, job)
	if err != nil || patch == nil {
		c.storeHandoff(ctx, task, job.ID, "builder", res.Handoff)
		why := "Builder finished without changing anything"
		if err != nil {
			why = err.Error()
		}
		return c.escalate(ctx, t, task, job, flow.Escalate, why)
	}
	head, err := c.push(ctx, task, job, patch, res.Summary)
	var protected = errors.Is(err, policy.ErrProtected)
	if protected {
		if _, aerr := q.AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
			RuleID: "constitution.protected", Category: "security", Severity: "block", LayerCaught: "L1",
			Message: err.Error() + ". Protected paths are changed only by Lead or learn in a separate PR.", Fingerprint: "constitution:" + job.Tenant,
		}); aerr != nil {
			return fmt.Errorf("conductor: %w", aerr)
		}
		c.appendEvent(ctx, task, eventPatchRejected, err.Error())
		return c.sendBack(ctx, t, task, job, res.Handoff, "the patch touches protected paths")
	}
	c.storeHandoff(ctx, task, job.ID, "builder", res.Handoff)
	if err != nil {
		return err
	}
	// Move first: a crash between the two leaves the job unconsumed, and checks() consumes it
	// before it sends the task back, so the old patch is never applied twice.
	if err = c.moveWithFindingUpdates(ctx, t, task, flow.PROpened, head, func(q *bookdb.Queries) error {
		return q.ResolveJobFindings(ctx, bookdb.ResolveJobFindingsParams{TaskID: task.ID, CheckedJobID: job.ID})
	}); err != nil {
		return err
	}
	return c.consumeDone(ctx, task)
}

// consumeDone marks every finished Builder job of the task as acted on.
func (c *Conductor) consumeDone(ctx context.Context, task bookdb.BookTask) error {
	q := bookdb.New(c.Pool)
	jobs, err := q.ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: task.ID, Role: "builder"})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	for _, j := range jobs {
		if _, err := q.ConsumeJob(ctx, j.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("conductor: %w", err)
		}
	}
	return nil
}

func (c *Conductor) appendEvent(ctx context.Context, task bookdb.BookTask, kind, note string) {
	if err := recordEvent(ctx, bookdb.New(c.Pool), task, kind, note); err != nil {
		c.log().WarnContext(ctx, "event not recorded", "task", task.ID, "kind", kind, "error", err)
	}
}

func recordEvent(ctx context.Context, q *bookdb.Queries, task bookdb.BookTask, kind, note string) error {
	payload, _ := json.Marshal(map[string]string{"note": note}) //nolint:errchkjson // a string map always encodes
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: kind, Actor: "system", Payload: payload,
	}); err != nil {
		return fmt.Errorf("conductor: record %s for task %d: %w", kind, task.ID, err)
	}
	return nil
}

// ofJob reports whether e was written by the consumption of job jobID.
func ofJob(e bookdb.BookEvent, jobID int64) bool {
	return e.JobID.Valid && e.JobID.Int64 == jobID
}

// recordJobEvent records an event of a job's consumption once. A consumption that failed is
// repeated on the next tick, and an agent that reads the task's events must not see it twice.
func recordJobEvent(ctx context.Context, q *bookdb.Queries, task bookdb.BookTask, jobID int64, kind, actor string, payload []byte) error {
	if _, err := q.AppendJobEvent(ctx, bookdb.AppendJobEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: kind, Actor: actor, Payload: payload,
		JobID: pgtype.Int8{Int64: jobID, Valid: true},
	}); err != nil {
		return fmt.Errorf("conductor: record %s of job %d: %w", kind, jobID, err)
	}
	return nil
}

// sendBack returns the task to Builder with the open findings, or escalates on the third
// red result of the layer.
func (c *Conductor) sendBack(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, handoff json.RawMessage, why string) error {
	// Claim the result and count its return together. A failed write leaves the
	// same job available on the next tick. Only its consumer starts the next job.
	var returns int64
	applied, err := c.consumeWith(ctx, job, func(q *bookdb.Queries) error {
		var err error
		returns, err = q.CountTaskEvents(ctx, bookdb.CountTaskEventsParams{
			TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventBuilderReturned,
		})
		if err != nil {
			return fmt.Errorf("conductor: count builder returns for task %d: %w", task.ID, err)
		}
		if returns >= maxL1Returns {
			return nil
		}
		return recordEvent(ctx, q, task, eventBuilderReturned, why)
	})
	if err != nil || !applied {
		return err
	}
	c.storeHandoff(ctx, task, job.ID, "builder", handoff)
	if returns >= maxL1Returns {
		return c.escalateTask(ctx, t, task, flow.BudgetExhausted, "the checks are still red after "+strconv.FormatInt(returns, 10)+" returns: "+why)
	}
	return c.enqueueBuilder(ctx, task)
}

func (c *Conductor) consumeWith(ctx context.Context, job bookdb.BookJob, record func(*bookdb.Queries) error) (bool, error) {
	var claimed bool
	err := pgx.BeginFunc(ctx, c.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		if _, err := q.ConsumeJob(ctx, job.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("conductor: consume job %d: %w", job.ID, err)
		}
		claimed = true
		return record(q)
	})
	return claimed && err == nil, err
}

// patchOf reads the job's patch from the artifacts volume and checks it against the hash the
// worker reported, so a file swapped after the job is refused. It returns nil for no patch.
func (c *Conductor) patchOf(ctx context.Context, job bookdb.BookJob) ([]byte, error) {
	arts, err := bookdb.New(c.Pool).ListJobArtifacts(ctx, job.ID)
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	for _, a := range arts {
		if !isPatchPath(a.Path) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.ArtifactDir, strconv.FormatInt(job.ID, 10), a.Path))
		if err != nil {
			return nil, fmt.Errorf("the patch of job %d is unreadable: %w", job.ID, err)
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != a.Sha256 {
			return nil, fmt.Errorf("the patch of job %d does not match its recorded hash", job.ID)
		}
		return data, nil
	}
	return nil, nil
}

// isPatchPath accepts the patch of one attempt, `<attempt>/patch.diff`, and the bare name that
// workers before per-attempt directories recorded. The attempt directory keeps a stale attempt
// of the same job from writing the file the core reads; the hash check stays the last word.
func isPatchPath(p string) bool {
	if p == jobsapi.PatchName {
		return true
	}
	dir, name := path.Split(p)
	dir = strings.TrimSuffix(dir, "/")
	return name == jobsapi.PatchName && dir != "" && dir != "." && dir != ".." && !strings.ContainsAny(dir, `/\`)
}

// push applies the patch to the job's base in nightshift's own clone, stores it as the
// task's branch, pushes it and opens the draft pull request the first time.
func (c *Conductor) push(ctx context.Context, task bookdb.BookTask, job bookdb.BookJob, patch []byte, summary string) (string, error) {
	var payload jobsapi.Job
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return "", fmt.Errorf("conductor: job %d payload: %w", job.ID, err)
	}
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return "", err
	}
	name := c.changeTitle(ctx, task)
	msg := name
	if summary != "" {
		msg += "\n\n" + summary
	}
	head, err := c.Git.ApplyPatch(ctx, mirror, branchOf(task), payload.BaseSHA, patch, c.Identity, msg)
	if err != nil {
		return "", err
	}
	if err := c.pushBranch(ctx, mirror, task.Repo, branchOf(task)); err != nil {
		return "", err
	}
	if prNumber(task) > 0 {
		return head, nil
	}
	ref, err := c.Forge.OpenPR(ctx, codehost.OpenPR{
		Repo: task.Repo, Head: branchOf(task), Base: c.base(), Title: name, Body: summary, Draft: true,
	})
	if err != nil {
		return "", fmt.Errorf("conductor: open pull request: %w", err)
	}
	if err := bookdb.New(c.Pool).SetTaskPR(ctx, bookdb.SetTaskPRParams{ID: task.ID, PrNumber: pgInt(ref.Number)}); err != nil {
		return "", fmt.Errorf("conductor: %w", err)
	}
	return head, nil
}

// enqueueBuilder queues the next Builder job: the task, the findings it must fix and the
// harness session of the earlier rounds.
func (c *Conductor) enqueueBuilder(ctx context.Context, task bookdb.BookTask) error {
	role, ok := c.Roles["builder"]
	if !ok {
		return errors.New("conductor: no builder role")
	}
	choice, err := c.Router.ChooseFor(ctx, "builder", c.fallbackOK(task))
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if choice.Decision != routing.Run {
		return nil // every provider is busy or limited; the next tick tries again
	}
	prov := c.Router.Providers[choice.Provider]
	model, effort := c.modelOf(role, prov)
	q := bookdb.New(c.Pool)
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return err
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	instructions, checklist, err := c.projectContext(ctx, mirror, base)
	if err != nil {
		return err
	}
	instructions, skills := c.withRoleOverride(ctx, mirror, base, "builder", instructions, role.Skills)
	job := jobsapi.Job{
		Traceparent:  routing.Traceparent(taskContext(ctx, task)),
		Instructions: instructions, Checklist: checklist,
		Harness: prov.Harness, Auth: prov.Auth, Model: model, Effort: effort, BaseURL: prov.BaseURL, KeyEnv: prov.KeyEnv, Price: priceOf(prov),
		Repo: task.Repo, BaseSHA: c.startFrom(ctx, mirror, task, base), OutputSchema: role.OutputSchema,
		Skills: skills, Budget: c.budget(role),
	}
	if s, err := q.GetHarnessSession(ctx, bookdb.GetHarnessSessionParams{TaskID: task.ID, Role: "builder"}); err == nil && s.Provider == choice.Provider {
		job.SessionID = s.SessionID
	}
	spec, request, handoff := c.specOf(ctx, task), c.requestOf(ctx, task), c.handoffOf(ctx, task, "builder")
	suffix := c.docsNote(ctx, task) + c.memoryNote(ctx, task.Tenant)
	return pgx.BeginFunc(ctx, c.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		canEnqueue, err := lockRoleForEnqueue(ctx, q, task, "builder")
		if err != nil || !canEnqueue {
			return err
		}
		all, err := q.ListTaskFindings(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("conductor: read findings for %d: %w", task.ID, err)
		}
		open, ids := findingsForBuilder(all)
		job.Prompt = builderPrompt(task, open, spec, request, handoff) + suffix
		payload, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
		next, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{
			Tenant: task.Tenant, TaskID: task.ID, Role: "builder", Provider: choice.Provider, Payload: payload,
		})
		if err != nil {
			return fmt.Errorf("conductor: enqueue: %w", err)
		}
		if err := q.HandOffFindings(ctx, bookdb.HandOffFindingsParams{Ids: ids, BuilderJobID: pgtype.Int8{Int64: next.ID, Valid: true}}); err != nil {
			return fmt.Errorf("conductor: record handoff to job %d: %w", next.ID, err)
		}
		return nil
	})
}

// findingsForBuilder carries unresolved blocking feedback into a retry. When a
// report repeats the same finding, include its unchanged prompt entry once.
func findingsForBuilder(all []bookdb.BookFindingState) ([]bookdb.BookFindingState, []int64) {
	var open []bookdb.BookFindingState
	var ids []int64
	type entry struct {
		layer, fingerprint, rule, category, path, message string
		start, end                                        pgtype.Int4
	}
	seen := map[entry]bool{}
	for _, f := range slices.Backward(all) {
		if f.Severity != "block" || (f.Status != "open" && f.Status != "handed_off") {
			continue
		}
		ids = append(ids, f.ID)
		key := entry{f.LayerCaught, f.Fingerprint, f.RuleID, f.Category, f.Path.String, f.Message, f.StartLine, f.EndLine}
		if f.Fingerprint == "" || !seen[key] {
			open = append(open, f)
			seen[key] = true
		}
	}
	slices.Reverse(open)
	return open, ids
}

func builderPrompt(task bookdb.BookTask, open []bookdb.BookFindingState, spec, request, handoff string) string {
	var b strings.Builder
	b.WriteString(task.Title)
	// The title is the first line of the owner's message, cut short. The rest of the message
	// (acceptance checks, constraints) is part of the task and goes along in full.
	if spec == "" && request != "" && request != task.Title {
		b.WriteString("\n\nThe task as the owner wrote it:\n" + request)
	}
	if spec != "" {
		b.WriteString("\n\nThe owner approved this spec:\n" + spec)
	}
	b.WriteString(previousSession(handoff))
	if len(open) > 0 {
		b.WriteString("\n\nThe last attempt left these findings. Fix each one, and answer a finding you think is wrong with a dispute:\n")
		for _, f := range open {
			fmt.Fprintf(&b, "- [%s] %s", f.RuleID, f.Message)
			if f.Path.Valid {
				fmt.Fprintf(&b, " (%s)", f.Path.String)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// projectContext reads the project's instruction files and the L0 checklist of its stacks
// from .nightshift/project.yaml at the base commit. A project without the file gets neither.
func (c *Conductor) projectContext(ctx context.Context, mirror, base string) (instructions, checklist []string, err error) {
	doc, ok, err := c.Git.Show(ctx, mirror, base, ".nightshift/project.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("conductor: %w", err)
	}
	if !ok {
		return nil, nil, nil
	}
	var p struct {
		Stack        []string `yaml:"stack"`
		Instructions []string `yaml:"instructions"`
	}
	if err := yaml.Unmarshal([]byte(doc), &p); err != nil {
		return nil, nil, fmt.Errorf("conductor: project.yaml: %w", err)
	}
	if c.Standards != nil {
		if checklist, err = standards.Checklist(c.Standards, p.Stack, jobsapi.StandardsDir); err != nil {
			return nil, nil, fmt.Errorf("conductor: %w", err)
		}
	}
	return p.Instructions, checklist, nil
}

// startFrom is the commit a Builder job works on. A task whose pull request is still open
// carries on from the tip of its branch, so a return round sees the earlier patch and the new
// patch lands on top of it; it used to start again from the base branch and was blind to its own
// work. The tip is read from the branch itself, not from the head in the book, which a merge-train
// rebase leaves behind. Anything else (no pull request, one that was merged or closed, as after a
// rollback, a branch the mirror no longer has, a lookup that failed) starts from the base.
func (c *Conductor) startFrom(ctx context.Context, mirror string, task bookdb.BookTask, base string) string {
	if prNumber(task) == 0 {
		return base
	}
	st, err := c.Forge.PRState(ctx, codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
	if err != nil || st.Merged || st.State != "open" {
		return base
	}
	tip, err := c.Git.Rev(ctx, mirror, branchOf(task))
	if err != nil {
		return base
	}
	return strings.TrimSpace(tip)
}
