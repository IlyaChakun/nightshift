package dispatch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

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
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return false, err
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

// askHuman holds the pull request: a pending status, the label and the reviewers. It asks once
// per head, because a host caps the statuses of a commit and the conductor ticks all night. A
// reviewer the host refuses (the author of the pull request) does not undo the hold.
func (c *Conductor) askHuman(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	asked, err := c.askedFor(ctx, task, ref.HeadSHA)
	if err != nil || asked {
		return err
	}
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
			c.log().WarnContext(ctx, "review not requested", "task", task.ID, "reviewers", who, "error", err)
		}
	}
	c.appendEvent(ctx, task, eventReviewAsked, ref.HeadSHA)
	return nil
}

func (c *Conductor) askedFor(ctx context.Context, task bookdb.BookTask, head string) (bool, error) {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	return slices.ContainsFunc(events, func(e bookdb.BookEvent) bool {
		return e.Kind == eventReviewAsked && strings.Contains(string(e.Payload), head)
	}), nil
}

// reviewers are the GitHub logins to ask: the owner and the approvers named by login. Teams and
// code owners come with the team path (phase 8).
func reviewers(t config.Tenant) []string {
	var who []string
	if owner := t.People.Owner.GitHub; owner != "" {
		who = append(who, owner)
	}
	for _, a := range t.People.Approvers {
		if login, ok := strings.CutPrefix(a, "github:"); ok && !slices.Contains(who, login) {
			who = append(who, login)
		}
	}
	return who
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
	latest := map[string]integration.Review{}
	for _, rv := range rvs {
		if rv.State == "commented" || !policy.CanDecideOnGitHub(t.People, rv.User) {
			continue
		}
		latest[rv.User] = rv // later reviews of the same person replace earlier ones
	}
	approved := false
	var requests []integration.Review
	for _, user := range slices.Sorted(maps.Keys(latest)) {
		switch rv := latest[user]; {
		// GitHub keeps a request for changes on the commit it was made on until the reviewer
		// looks again; once Builder pushed, that review is stale and must not loop the task.
		case rv.State == "changes_requested" && rv.CommitSHA == ref.HeadSHA:
			requests = append(requests, rv)
		case rv.State == "approved" && rv.CommitSHA == ref.HeadSHA:
			approved = true
		}
	}
	if len(requests) > 0 {
		return c.changesRequested(ctx, t, task, ref, requests)
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

// maxReviewText bounds what a review adds to Builder's prompt.
const maxReviewText = 4000

// reviewFinding is what Builder reads about a request for changes. It has no access to the
// code host, so the reviewer's words travel with the finding, masked like chat text.
func reviewFinding(rv integration.Review) string {
	text := []rune(strings.TrimSpace(NewMasker().String(rv.Body)))
	if len(text) == 0 {
		return rv.User + " requested changes on the pull request and wrote no comment."
	}
	if len(text) > maxReviewText {
		text = append(text[:maxReviewText], []rune("… (cut)")...)
	}
	return rv.User + " requested changes on the pull request:\n" + string(text)
}

func (c *Conductor) changesRequested(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref integration.PRRef, requests []integration.Review) error {
	// All comments are read before anything is stored: a failed call is retried on the next
	// tick, and Builder must neither start without them nor get a finding twice.
	for i, rv := range requests {
		lines, err := c.Forge.LineComments(ctx, ref, rv.ID)
		if err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
		requests[i].Body = strings.TrimSpace(rv.Body + lines)
	}
	for _, rv := range requests {
		if _, err := bookdb.New(c.Pool).AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, RuleID: "review.changes_requested", Category: "review", Severity: "block",
			LayerCaught: "L4", Message: reviewFinding(rv),
			Fingerprint: "review:" + rv.User + ":" + rv.CommitSHA,
		}); err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
	}
	task, err := c.move(ctx, t, task, flow.RequestChanges, "")
	if err != nil {
		return err
	}
	return c.building(ctx, t, task)
}

// outside follows a pull request that was merged or closed on the code host while the task
// waited for a person: a merge ends the task as merged by someone else, a close cancels it,
// and both are events of the book. It reports whether the task moved.
func (c *Conductor) outside(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	if prNumber(task) == 0 {
		return false, nil
	}
	st, err := c.Forge.PRState(ctx, integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	var ev flow.Event
	var what string
	switch {
	case st.Merged:
		ev, what = flow.MergedOutside, "merged"
	case st.State == "closed":
		ev, what = flow.ClosedOutside, "closed"
	default:
		return false, nil
	}
	moved, err := c.move(ctx, t, task, ev, "")
	if err != nil {
		return false, err
	}
	c.tellOutside(ctx, moved, what, st.URL)
	return true, nil
}

// tellOutside tells the owner that a task ended because its pull request was merged or closed
// outside the factory. Without a chat thread (a task added from the terminal) the event in the
// book is the record.
func (c *Conductor) tellOutside(ctx context.Context, task bookdb.BookTask, what, url string) {
	ref, err := threadOf(task)
	if err != nil || c.Outbox == nil {
		return
	}
	text := fmt.Sprintf("Task #%d ended: its pull request was %s outside the factory (%s).", task.ID, what, url)
	if err := c.postCard(ctx, bookdb.New(c.Pool), task, integration.CardBlocker, text, nil, ref, fmt.Sprintf("outside:%d:%s", task.ID, what)); err != nil {
		c.log().WarnContext(ctx, "card about an outside merge not queued", "task", task.ID, "error", err)
	}
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
	mirror, err := c.mirror(ctx, repo)
	if err != nil {
		return gitops.Rebased{}, err
	}
	res, err := c.Git.Rebase(ctx, mirror, branch, c.base(), c.Identity)
	if err != nil || res.Conflict {
		return res, err
	}
	if err := c.pushBranch(ctx, mirror, repo, branch); err != nil {
		return gitops.Rebased{}, err
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
