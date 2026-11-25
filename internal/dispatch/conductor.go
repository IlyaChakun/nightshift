package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/train"
)

// maxL1Returns is how many times checks before the push send a task back to Builder; the
// next red result escalates instead (design section 8, two automatic rounds per layer).
const maxL1Returns = 2

// Event kinds the conductor writes besides task.moved.
const (
	eventBuilderReturned = "builder.returned"
	eventPatchRejected   = "patch.rejected"
	eventReviewDecided   = "review.decided"
	eventRaised          = "review.raised"
	eventReviewAsked     = "review.asked"
)

// Conductor drives a task from `queued` through Builder, the pre-push checks, the pull
// request and CI. Advance is safe to call again and again: every step either finds its
// work already done in the book or does it once, so a restart or a second tick repeats
// nothing. Side effects (git, GitHub) happen here, in nightshift, never in an agent.
type Conductor struct {
	Pool   *pgxpool.Pool
	Mover  *flow.Mover
	Router *Router
	Roles  map[string]config.Role
	Git    *gitops.Git
	Forge  integration.PullRequests
	Remote func(repo string) string // clone URL of a repository
	// Auth is the credential of git for a repository; nil means none, as on a local remote.
	Auth        func(ctx context.Context, repo string) (gitops.Auth, error)
	ArtifactDir string // where the worker writes job artifacts, mounted read-only
	Identity    gitops.Identity
	BaseBranch  string // defaults to main
	// Rules classify a change by blast radius; Roll draws the number for review sampling.
	// Without Rules every change goes to a person.
	Rules *policy.Rules
	// Validator reads `.nightshift/blast-radius.yaml` of the repository when Rules is nil.
	Validator *config.Validator
	Roll      func() float64
	Train     *train.Train
	// Standards is the standards pack (the `standards` directory); without it the L0
	// checklist is empty.
	Standards fs.FS
	// Outbox carries the Lead stage's messages to chat; without it the stage stays off.
	Outbox *Outbox
	// Fallback is the tenant's on_limit.fallback: never, per_task (the default) or always.
	Fallback string
	Log      *slog.Logger
}

// fallbackOK says whether the task may run on a fallback provider (design section 12).
func (c *Conductor) fallbackOK(task bookdb.BookTask) bool {
	switch c.Fallback {
	case "never":
		return false
	case "always":
		return true
	}
	return task.AllowFallback
}

func (c *Conductor) base() string {
	if c.BaseBranch != "" {
		return c.BaseBranch
	}
	return "main"
}

func (c *Conductor) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

func (c *Conductor) profile(t config.Tenant) flow.Profile {
	if t.Governance == "team" {
		return flow.Team
	}
	return flow.Solo
}

func (c *Conductor) move(ctx context.Context, t config.Tenant, task bookdb.BookTask, e flow.Event, head string) (bookdb.BookTask, error) {
	return c.Mover.Apply(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: "system", Profile: c.profile(t), HeadSHA: head})
}

// Advance does the next thing the task is waiting for, if there is one. A lost race with
// another decision (flow.ErrConflict) is not an error: the next tick looks again.
func (c *Conductor) Advance(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	var err error
	switch flow.State(task.State) { //nolint:exhaustive // the other states belong to Lead, to people or to later phases
	case flow.Idea, flow.Grilling:
		err = c.leading(ctx, t, task)
	case flow.Queued:
		if task.NotBefore.Valid && task.NotBefore.Time.After(time.Now()) {
			return nil // planned for later, for example the night
		}
		if err = c.expandSpec(ctx, task); err != nil {
			return err
		}
		gate, broken, derr := c.depsGate(ctx, task)
		switch {
		case derr != nil:
			err = derr
		case gate == depsBroken:
			err = c.blockOnDeps(ctx, t, task, broken)
		case gate == depsReady:
			if task, err = c.move(ctx, t, task, flow.DepsClosed, ""); err == nil {
				err = c.building(ctx, t, task)
			}
		}
	case flow.Building:
		err = c.building(ctx, t, task)
	case flow.Checks:
		err = c.checks(ctx, t, task)
	case flow.Review, flow.Preview, flow.Acceptance:
		err = c.gate(ctx, t, task)
	case flow.HumanReview:
		err = c.humanReview(ctx, t, task)
	case flow.Ready:
		err = c.ready(ctx, t, task)
	case flow.HandedOff:
		err = c.handedOff(ctx, t, task)
	case flow.Merging:
		err = c.merging(ctx, t, task)
	}
	if errors.Is(err, flow.ErrConflict) {
		return nil
	}
	return err
}

// checks reads CI of the pushed head (L3): green moves the task on, red hands the failing
// checks to Builder as a finding.
func (c *Conductor) checks(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	if !task.HeadSha.Valid {
		return fmt.Errorf("conductor: task %d is in checks without a head", task.ID)
	}
	if err := c.consumeDone(ctx, task); err != nil {
		return err
	}
	res, err := c.Forge.CI(ctx, task.Repo, task.HeadSha.String)
	if err != nil {
		return fmt.Errorf("conductor: ci of task %d: %w", task.ID, err)
	}
	switch res.State {
	case integration.CIPending:
		// Nothing to do until CI reports.
	case integration.CISuccess:
		_, err = c.move(ctx, t, task, flow.AllGreen, "")
	case integration.CIFailure:
		msg := "CI failed: " + strings.Join(res.Failing, ", ")
		_, err = bookdb.New(c.Pool).AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, RuleID: "ci.failed", Category: "tests",
			Severity: "block", LayerCaught: "L3", Message: msg, Fingerprint: "ci:" + strings.Join(res.Failing, ","),
		})
		if err == nil {
			// Builder starts on the same tick: the findings are fresh.
			if task, err = c.move(ctx, t, task, flow.Findings, ""); err == nil {
				err = c.building(ctx, t, task)
			}
		}
	}
	return err
}

// prNumber is the pull request of the task, zero when none was opened yet.
func prNumber(task bookdb.BookTask) int {
	if !task.PrNumber.Valid {
		return 0
	}
	return int(task.PrNumber.Int32)
}

func branchOf(task bookdb.BookTask) string { return "ns/" + strconv.FormatInt(task.ID, 10) }

func pgInt(n int) pgtype.Int4 { return pgtype.Int4{Int32: int32(n), Valid: true} } //nolint:gosec // pull request numbers fit in 32 bits
