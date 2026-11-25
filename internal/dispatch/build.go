package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/runner"
	"github.com/ilyachakun/nightshift/internal/standards"
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
		return c.escalate(ctx, t, task, last, "the Builder job ended with "+last.ExitReason.String)
	}
}

// escalate ends a job and sends the task to `escalated` for the owner to decide.
func (c *Conductor) escalate(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, why string) error {
	if _, err := bookdb.New(c.Pool).ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	c.log().WarnContext(ctx, "task escalated", "task", task.ID, "why", why)
	_, err := c.move(ctx, t, task, flow.BudgetExhausted, "")
	return err
}

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
		return c.escalate(ctx, t, task, job, "the Builder result is not JSON")
	}
	c.storeHandoff(ctx, task, "builder", res.Handoff)
	if res.Status == "blocked" {
		if back, err := c.backToLead(ctx, t, task, job, res.BlockedReason); back || err != nil {
			return err
		}
		return c.escalate(ctx, t, task, job, "Builder is blocked: "+res.BlockedReason)
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
		return c.sendBack(ctx, t, task, job, "the checks found problems")
	}
	patch, err := c.patchOf(ctx, job)
	if err != nil {
		return c.escalate(ctx, t, task, job, err.Error())
	}
	if patch == nil {
		return c.escalate(ctx, t, task, job, "Builder finished without changing anything")
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
		return c.sendBack(ctx, t, task, job, "the patch touches protected paths")
	}
	if err != nil {
		return err
	}
	// Move first: a crash between the two leaves the job unconsumed, and checks() consumes it
	// before it sends the task back, so the old patch is never applied twice.
	if _, err = c.move(ctx, t, task, flow.PROpened, head); err != nil {
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
	payload, _ := json.Marshal(map[string]string{"note": note}) //nolint:errchkjson // a string map always encodes
	if _, err := bookdb.New(c.Pool).AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: kind, Actor: "system", Payload: payload,
	}); err != nil {
		c.log().WarnContext(ctx, "event not recorded", "task", task.ID, "kind", kind, "error", err)
	}
}

// sendBack returns the task to Builder with the open findings, or escalates on the third
// red result of the layer.
func (c *Conductor) sendBack(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, why string) error {
	q := bookdb.New(c.Pool)
	n, err := q.CountTaskEvents(ctx, bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventBuilderReturned})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if n >= maxL1Returns {
		return c.escalate(ctx, t, task, job, "the checks are still red after "+strconv.FormatInt(n, 10)+" returns: "+why)
	}
	c.appendEvent(ctx, task, eventBuilderReturned, why)
	if _, err := q.ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	return c.enqueueBuilder(ctx, task)
}

// patchOf reads the job's patch from the artifacts volume and checks it against the hash the
// worker reported, so a file swapped after the job is refused. It returns nil for no patch.
func (c *Conductor) patchOf(ctx context.Context, job bookdb.BookJob) ([]byte, error) {
	arts, err := bookdb.New(c.Pool).ListJobArtifacts(ctx, job.ID)
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	for _, a := range arts {
		if a.Path != runner.PatchName {
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
	ref, err := c.Forge.OpenPR(ctx, integration.OpenPR{
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
	if choice.Decision != Run {
		return nil // every provider is busy or limited; the next tick tries again
	}
	prov := c.Router.Providers[choice.Provider]
	q := bookdb.New(c.Pool)
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return err
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	open, err := c.openFindings(ctx, task)
	if err != nil {
		return err
	}
	instructions, checklist, err := c.projectContext(ctx, mirror, base)
	if err != nil {
		return err
	}
	instructions, skills := c.withRoleOverride(ctx, mirror, base, "builder", instructions, role.Skills)
	job := jobsapi.Job{
		Traceparent:  Traceparent(taskContext(ctx, task)),
		Instructions: instructions, Checklist: checklist,
		Harness: prov.Harness, Auth: prov.Auth, Model: prov.Model, BaseURL: prov.BaseURL, KeyEnv: prov.KeyEnv, Price: priceOf(prov),
		Repo: task.Repo, BaseSHA: base, Prompt: builderPrompt(task, open, c.specOf(ctx, task), c.requestOf(ctx, task), c.handoffOf(ctx, task, "builder")) + c.memoryNote(ctx, task.Tenant), OutputSchema: role.OutputSchema,
		Skills: skills, Budget: jobsapi.Budget{Tokens: role.Limits.Tokens, Minutes: role.Limits.Minutes},
	}
	if s, err := q.GetHarnessSession(ctx, bookdb.GetHarnessSessionParams{TaskID: task.ID, Role: "builder"}); err == nil && s.Provider == choice.Provider {
		job.SessionID = s.SessionID
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if _, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{
		Tenant: task.Tenant, TaskID: task.ID, Role: "builder", Provider: choice.Provider, Payload: payload,
	}); err != nil {
		return fmt.Errorf("conductor: enqueue: %w", err)
	}
	// Handed over only now: a failure above leaves them open for the next tick.
	for _, f := range open {
		if _, err := q.SetFindingStatus(ctx, bookdb.SetFindingStatusParams{ID: f.ID, Status: "fixed"}); err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
	}
	return nil
}

// openFindings are the blocking findings Builder has not been told about yet. Once they are in
// a job they count as handed over; the next check run reports what is still wrong.
func (c *Conductor) openFindings(ctx context.Context, task bookdb.BookTask) ([]bookdb.BookFinding, error) {
	q := bookdb.New(c.Pool)
	all, err := q.ListTaskFindings(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	var open []bookdb.BookFinding
	for _, f := range all {
		if f.Severity != "block" || f.Status != "open" {
			continue
		}
		open = append(open, f)
	}
	return open, nil
}

func builderPrompt(task bookdb.BookTask, open []bookdb.BookFinding, spec, request, handoff string) string {
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
		if checklist, err = standards.Checklist(c.Standards, p.Stack); err != nil {
			return nil, nil, fmt.Errorf("conductor: %w", err)
		}
	}
	return p.Instructions, checklist, nil
}
