package dispatch

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/train"
)

// humanReviewStatus is the commit status that keeps a pull request unmergeable until a
// person has looked at it (design section 11).
const humanReviewStatus = "nightshift/human-review"

// gate takes a task through review, preview and acceptance. The review step runs the
// Verifier when the tenant has one (verify.go); without a Verifier, and for preview and
// acceptance (no preview environment yet), the step passes without a verdict. The review
// policy still decides whether a person must look.
func (c *Conductor) gate(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	if flow.State(task.State) == flow.Review {
		if handled, err := c.verifying(ctx, t, task); handled || err != nil {
			return err
		}
	}
	needs, err := c.needsHuman(ctx, t, task)
	if err != nil {
		return err
	}
	for _, step := range []struct {
		from flow.State
		ev   flow.Event
	}{{flow.Review, flow.VerifierOK}, {flow.Preview, flow.EnvUp}, {flow.Acceptance, flow.Accepted}} {
		if flow.State(task.State) != step.from {
			continue
		}
		mv := flow.Move{TaskID: task.ID, Event: step.ev, Actor: "system", Profile: c.profile(t), NeedsHumanReview: needs}
		if task, err = c.Mover.Apply(ctx, mv); err != nil {
			return err
		}
	}
	if flow.State(task.State) == flow.HumanReview {
		return c.askHuman(ctx, t, task)
	}
	return nil
}

// needsHuman classifies the change from the branch itself and applies the tenant's review
// policy. Without rules or a policy every change is held for a person: the safe side.
func (c *Conductor) needsHuman(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	if t.Review.Mode == "" {
		return true, nil
	}
	// A Verifier that raised the blast radius holds the change for a person, whatever the policy says.
	raised, err := bookdb.New(c.Pool).CountTaskEvents(ctx, bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventRaised})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	if raised > 0 {
		return true, nil
	}
	mirror, err := c.Git.EnsureMirror(ctx, task.Repo, c.Remote(task.Repo), c.Auth)
	if err != nil {
		return false, fmt.Errorf("conductor: mirror of %s: %w", task.Repo, err)
	}
	rules, err := c.rulesOf(ctx, mirror)
	if err != nil || rules == nil {
		return true, err
	}
	paths, lines, err := c.Git.Changes(ctx, mirror, c.base(), branchOf(task))
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	cls := rules.Classify(policy.Change{Paths: paths, DiffLines: lines})
	actions := map[policy.Level]policy.Action{}
	for l, a := range t.Review.Actions {
		actions[policy.Level(l)] = policy.Action(a)
	}
	pol := policy.ReviewPolicy{Mode: policy.Mode(t.Review.Mode), Team: t.Governance == "team", Actions: actions, Sample: t.Review.Sample}
	roll := 1.0
	if c.Roll != nil {
		roll = c.Roll()
	}
	act, err := pol.Decide(cls.Level, roll)
	if err != nil {
		return true, nil //nolint:nilerr // a policy that cannot decide holds the change for a person
	}
	c.appendEvent(ctx, task, eventReviewDecided, string(cls.Level)+" -> "+string(act))
	return act.NeedsHuman(), nil
}

// askHuman holds the pull request: a pending status, the label and the reviewers. It is
// safe to repeat; the twin and GitHub both accept the same status, label and reviewers again.
func (c *Conductor) askHuman(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	if err := c.Forge.SetStatus(ctx, task.Repo, ref.HeadSHA, integration.CommitStatus{
		Context: humanReviewStatus, State: "pending", Description: "waiting for a person to review",
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if err := c.Forge.AddLabels(ctx, ref, []string{"nightshift/human-review"}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if who := reviewers(t); len(who) > 0 {
		if err := c.Forge.RequestReviewers(ctx, ref, who); err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
	}
	return nil
}

func reviewers(t config.Tenant) []string {
	var who []string
	if t.People.Owner.GitHub != "" {
		who = append(who, t.People.Owner.GitHub)
	}
	return append(who, t.People.Approvers...)
}

// humanReview reads the reviews of the current head. An approval from the owner or an
// approver clears the hold; a request for changes goes to Builder as a finding.
func (c *Conductor) humanReview(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	if err := c.askHuman(ctx, t, task); err != nil {
		return err
	}
	rvs, err := c.Forge.Reviews(ctx, ref)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	allowed := reviewers(t)
	latest := map[string]integration.Review{}
	for _, rv := range rvs {
		if rv.State == "commented" || !slices.Contains(allowed, rv.User) {
			continue
		}
		latest[rv.User] = rv // later reviews of the same person replace earlier ones
	}
	approved := false
	for _, rv := range latest {
		switch {
		case rv.State == "changes_requested":
			return c.changesRequested(ctx, t, task, rv)
		case rv.State == "approved" && rv.CommitSHA == ref.HeadSHA:
			approved = true
		}
	}
	if !approved {
		return nil
	}
	if err := c.Forge.SetStatus(ctx, task.Repo, ref.HeadSHA, integration.CommitStatus{
		Context: humanReviewStatus, State: "success", Description: "reviewed by a person",
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	_, err = c.move(ctx, t, task, flow.Approve, "")
	return err
}

func (c *Conductor) changesRequested(ctx context.Context, t config.Tenant, task bookdb.BookTask, rv integration.Review) error {
	if _, err := bookdb.New(c.Pool).AddFinding(ctx, bookdb.AddFindingParams{
		Tenant: task.Tenant, TaskID: task.ID, RuleID: "review.changes_requested", Category: "review", Severity: "block",
		LayerCaught: "L4", Message: rv.User + " requested changes on the pull request; read their comments there.",
		Fingerprint: "review:" + rv.User + ":" + rv.CommitSHA,
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	task, err := c.move(ctx, t, task, flow.RequestChanges, "")
	if err != nil {
		return err
	}
	return c.building(ctx, t, task)
}

// ready is the hand-over after acceptance. Solo: take the pull request out of draft and
// start the merge train. Team: hand the pull request to the team, which merges it.
func (c *Conductor) ready(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	if err := c.Forge.MarkReady(ctx, ref); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if c.profile(t) == flow.Team {
		_, err := c.move(ctx, t, task, flow.HandOff, "")
		return err
	}
	task, err := c.move(ctx, t, task, flow.Merge, "")
	if err != nil {
		return err
	}
	return c.merging(ctx, t, task)
}

// handedOff watches a pull request the team owns.
func (c *Conductor) handedOff(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	st, err := c.Forge.PRState(ctx, integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	switch {
	case st.Merged:
		_, err = c.move(ctx, t, task, flow.TeamMerged, "")
	case st.State == "closed":
		_, err = c.move(ctx, t, task, flow.PRClosed, "")
	}
	return err
}

// merging runs one step of the train for the task.
func (c *Conductor) merging(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	if c.Train == nil {
		return errors.New("conductor: no merge train")
	}
	_, err := c.Train.Run(ctx, train.Job{
		Tenant: task.Tenant, TaskID: task.ID, Branch: branchOf(task), Profile: c.profile(t),
		PR: integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String},
	})
	if errors.Is(err, train.ErrClosed) {
		_, err = c.move(ctx, t, task, flow.Cancel, "")
	}
	return err
}

// Rebase lets the conductor serve as the train's Rebaser: it rebases the task branch on the
// base branch in nightshift's own clone and pushes the result.
func (c *Conductor) Rebase(ctx context.Context, repo, branch string) (gitops.Rebased, error) {
	mirror, err := c.Git.EnsureMirror(ctx, repo, c.Remote(repo), c.Auth)
	if err != nil {
		return gitops.Rebased{}, fmt.Errorf("conductor: mirror of %s: %w", repo, err)
	}
	res, err := c.Git.Rebase(ctx, mirror, branch, c.base(), c.Identity)
	if err != nil || res.Conflict {
		return res, err
	}
	if err := c.Git.Push(ctx, mirror, c.Remote(repo), branch, c.Auth); err != nil {
		return gitops.Rebased{}, fmt.Errorf("conductor: push: %w", err)
	}
	return res, nil
}

// rulesOf is the injected rules, or the repository's own file at the base branch. A
// repository without the file has no rules, and then every change goes to a person.
func (c *Conductor) rulesOf(ctx context.Context, mirror string) (*policy.Rules, error) {
	if c.Rules != nil || c.Validator == nil {
		return c.Rules, nil
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	doc, ok, err := c.Git.Show(ctx, mirror, base, ".nightshift/blast-radius.yaml")
	if err != nil || !ok {
		return nil, err
	}
	rules, err := policy.ParseRules([]byte(doc), c.Validator)
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	return rules, nil
}
