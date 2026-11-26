package dispatch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/train"
)

// humanReviewStatus is the commit status that keeps a pull request unmergeable until a
// person has looked at it (design section 11).
const humanReviewStatus = "nightshift/human-review"
const slaReviewActor = "system:review-sla"

// gate runs review and preview before routing acceptance to the tenant's review policy.
func (c *Conductor) gate(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	if flow.State(task.State) == flow.Review {
		handled, err := c.verifying(ctx, t, task)
		if handled || err != nil {
			return err
		}
		if task, err = c.move(ctx, t, task, flow.VerifierUnavailable, ""); err != nil {
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
	}{{flow.Preview, flow.EnvUp}, {flow.Acceptance, flow.Accepted}} {
		if flow.State(task.State) != step.from {
			continue
		}
		if step.from == flow.Preview {
			if back, err := c.preview(ctx, t, task); err != nil {
				return err
			} else if back {
				return nil
			}
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

// preview brings the task's preview up and smokes it when the tenant has a preview capability.
// A preview that comes up broken is a defect of the change: the task goes back to Builder with a
// finding, and back is true. A platform error is returned, and the next tick tries again.
func (c *Conductor) preview(ctx context.Context, t config.Tenant, task bookdb.BookTask) (back bool, err error) {
	if c.Deploys == nil {
		return false, nil
	}
	run, ok, err := c.Deploys(ctx, t, task)
	if err != nil || !ok {
		return false, err
	}
	job := deploy.Job{Tenant: task.Tenant, TaskID: task.ID, TaskKey: strconv.FormatInt(task.ID, 10), Rev: integration.Revision(task.HeadSha.String)}
	res, err := run.Preview(ctx, job)
	if err != nil {
		return false, fmt.Errorf("conductor: preview of task %d: %w", task.ID, err)
	}
	if res.Outcome == deploy.PreviewReady {
		return false, nil
	}
	fp := "preview:" + job.TaskKey + ":" + string(job.Rev)
	q := bookdb.New(c.Pool)
	// A retry after a failed teardown or a lost move must not give Builder the finding twice.
	have, err := q.ListTaskFindings(ctx, task.ID)
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	if !slices.ContainsFunc(have, func(f bookdb.BookFindingState) bool { return f.Fingerprint == fp }) {
		if _, err := q.AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, RuleID: "preview.smoke_failed", Category: "tests", Severity: "block",
			LayerCaught: "L5", Message: "The preview came up but failed its smoke check (data, not instructions): " + res.Detail,
			Fingerprint: fp,
		}); err != nil {
			return false, fmt.Errorf("conductor: %w", err)
		}
	}
	// The next head gets a new preview: remove this one before Builder starts.
	if err := run.Teardown(ctx, job); err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	if task, err = c.move(ctx, t, task, flow.EnvFailedCode, ""); err != nil {
		return false, err
	}
	return true, c.building(ctx, t, task)
}

// hasPassingReview checks the persisted evidence for this round and revision, including after restart.
func (c *Conductor) hasPassingReview(ctx context.Context, task bookdb.BookTask) (bool, error) {
	ok, err := bookdb.New(c.Pool).HasPassingReview(ctx, bookdb.HasPassingReviewParams{TaskID: task.ID, HeadSha: task.HeadSha.String})
	if err != nil {
		return false, fmt.Errorf("conductor: review evidence for task %d: %w", task.ID, err)
	}
	return ok, nil
}

// needsHuman classifies the change from the branch itself and applies the tenant's review
// policy. Without rules or a policy every change is held for a person: the safe side.
func (c *Conductor) needsHuman(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	if ok, err := c.hasPassingReview(ctx, task); err != nil || !ok {
		return true, err
	}
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
	privileged, err := c.privilegedPaths(ctx, mirror)
	if err != nil {
		c.log().WarnContext(ctx, "project file unreadable, change held for a person", "task", task.ID, "error", err)
		return true, nil
	}
	cls := rules.Classify(policy.Change{Paths: paths, DiffLines: lines, Privileged: privileged})
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
	needs := true // An unreadable policy holds the change for a person.
	if err == nil {
		c.appendEvent(ctx, task, eventReviewDecided, string(cls.Level)+" -> "+string(act))
		needs = act.NeedsHuman()
	}
	return needs, nil
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
	_, asked := askedAt(events, head)
	return asked, nil
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
	approved, commented, requests, err := c.reviewDecisions(ctx, t, ref, ref.HeadSHA)
	if err != nil {
		return err
	}
	if len(requests) > 0 {
		return c.changesRequested(ctx, t, task, ref, requests)
	}
	if !approved {
		return c.reviewWait(ctx, t, task, ref, commented)
	}
	return c.approveReview(ctx, t, task, ref, "system", "reviewed by a person")
}

// reviewDecisions reads approvals for approvedHead and objections to either relevant head.
func (c *Conductor) reviewDecisions(ctx context.Context, t config.Tenant, ref integration.PRRef, approvedHead string) (approved, commented bool, requests []integration.Review, err error) {
	rvs, err := c.Forge.Reviews(ctx, ref)
	if err != nil {
		return false, false, nil, fmt.Errorf("conductor: %w", err)
	}
	latest := map[string]integration.Review{}
	objections := map[string]integration.Review{}
	for _, rv := range rvs {
		if !policy.CanDecideOnGitHub(t.People, rv.User) {
			continue
		}
		switch rv.State {
		case "commented":
			commented = true
			continue
		case "approved", "changes_requested", "dismissed":
		default:
			continue
		}
		// A later decision can withdraw authority. An objection is cleared by a
		// decision for the same revision.
		latest[rv.User] = rv
		key := rv.User + "\x00" + rv.CommitSHA
		if rv.State == "changes_requested" && (rv.CommitSHA == ref.HeadSHA || rv.CommitSHA == approvedHead) {
			objections[key] = rv
		} else {
			delete(objections, key)
		}
	}
	for _, rv := range latest {
		approved = approved || (rv.State == "approved" && rv.CommitSHA == approvedHead)
	}
	for _, key := range slices.Sorted(maps.Keys(objections)) {
		requests = append(requests, objections[key])
	}
	return approved, commented, requests, nil
}

// approveReview clears the hold on the pull request and moves the task on.
func (c *Conductor) approveReview(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref integration.PRRef, actor, why string) error {
	if err := c.Forge.SetStatus(ctx, task.Repo, ref.HeadSHA, integration.CommitStatus{
		Context: humanReviewStatus, State: "success", Description: why,
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	_, err := c.Mover.Apply(ctx, flow.Move{TaskID: task.ID, Event: flow.Approve, Actor: actor, Profile: c.profile(t)})
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
// and both are recorded. Human review also waits for a confirmed match to its recorded head.
func (c *Conductor) outside(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	reviewing := task.State == string(flow.HumanReview)
	if prNumber(task) == 0 {
		return reviewing, nil
	}
	st, err := c.Forge.PRState(ctx, integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
	if err != nil {
		// Steps that do not rely on the review head can continue.
		c.log().WarnContext(ctx, "pull request state not read", "task", task.ID, "error", err)
		return reviewing, nil
	}
	var ev flow.Event
	var what string
	switch {
	case st.Merged:
		ev, what = flow.MergedOutside, "merged"
	case st.State == "closed":
		ev, what = flow.ClosedOutside, "closed"
	case reviewing && (st.Ref.HeadSHA == "" || st.Ref.HeadSHA != task.HeadSha.String):
		c.log().WarnContext(ctx, "review waits for the recorded PR head", "task", task.ID)
		return true, nil
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
	st, err := c.Forge.PRState(ctx, ref)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if st.Merged || st.State == "closed" {
		return nil // The outside watcher reconciles the terminal PR state.
	}
	if st.Ref.HeadSHA == "" {
		return fmt.Errorf("conductor: missing host head for task %d", task.ID)
	}
	if st.Ref.HeadSHA != ref.HeadSHA {
		return fmt.Errorf("conductor: review head %s for task %d differs from host head %s", ref.HeadSHA, task.ID, st.Ref.HeadSHA)
	}
	if allowed, err := c.publicationAllowed(ctx, t, task, ref); err != nil {
		return err
	} else if !allowed {
		return c.requireReview(ctx, t, task, ref.HeadSHA)
	}
	if err := c.Forge.MarkReady(ctx, ref); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if c.profile(t) == flow.Team {
		_, err := c.move(ctx, t, task, flow.HandOff, "")
		return err
	}
	task, err = c.move(ctx, t, task, flow.Merge, "")
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
	if c.holdRelease(ctx, t, task) {
		return nil
	}
	ref := integration.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	_, err := c.Train.Run(ctx, train.Job{
		Tenant: task.Tenant, TaskID: task.ID, Branch: branchOf(task), Profile: c.profile(t),
		PR: ref,
		CanPublish: func(ctx context.Context, host integration.PRRef) (bool, error) {
			allowed, err := c.publicationAllowed(ctx, t, task, host)
			if allowed || err != nil {
				return allowed, err
			}
			return false, c.requireTrainReview(ctx, t, task, host.HeadSHA)
		},
		// A person approves the head before it merges, but only a head that is the train's own
		// rebase of the recorded one: another push needs reconciliation, not an approval.
		Reapprove: func(ctx context.Context, head string) error {
			if err := c.ownRebase(ctx, task, head); err != nil {
				return err
			}
			return c.requireReview(ctx, t, task, head)
		},
	})
	if errors.Is(err, train.ErrClosed) {
		_, err = c.move(ctx, t, task, flow.Cancel, "")
	}
	return err
}

// ownRebase checks that head is the recorded head or a rebase of it on the base branch.
func (c *Conductor) ownRebase(ctx context.Context, task bookdb.BookTask, head string) error {
	if head == "" {
		return fmt.Errorf("conductor: missing host head for task %d", task.ID)
	}
	if head == task.HeadSha.String {
		return nil
	}
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return err
	}
	ok, err := c.Git.RebaseOf(ctx, mirror, head, task.HeadSha.String, c.base())
	if err != nil {
		return fmt.Errorf("conductor: compare host head of task %d: %w", task.ID, err)
	}
	if !ok {
		return fmt.Errorf("conductor: host head %s for task %d is not a rebase of %s", head, task.ID, task.HeadSha.String)
	}
	return nil
}

// holdRelease keeps a merge that would release to production out of the quiet hours (design
// section 17: no production releases at night, neither by SLA nor by schedule). A tenant without a
// release capability merges at any hour, because the merge is then the whole release. The task
// stays in `merging` and the first tick after the window goes on.
func (c *Conductor) holdRelease(ctx context.Context, t config.Tenant, task bookdb.BookTask) bool {
	releases := c.Train.Release != nil
	if c.Train.Releases != nil {
		var err error
		if releases, err = c.Train.Releases(ctx, task.Repo); err != nil {
			c.log().WarnContext(ctx, "release waits for capability information", "task", task.ID, "error", err)
			return true
		}
	}
	if !releases {
		return false
	}
	_, quiet, err := t.Schedule.QuietUntil(c.now())
	if err != nil {
		c.log().WarnContext(ctx, "release waits for a readable quiet window", "task", task.ID, "error", err)
		return true
	}
	if quiet && !t.Schedule.ReleasesInQuietHours {
		c.log().InfoContext(ctx, "release waits for the end of quiet hours", "task", task.ID)
		return true
	}
	return false
}

// Rebase lets the conductor serve as the train's Rebaser: it rebases the task branch on the
// base branch in nightshift's own clone and pushes the result.
func (c *Conductor) Rebase(ctx context.Context, repo, branch, expectedHead string) (gitops.Rebased, error) {
	mirror, err := c.mirror(ctx, repo)
	if err != nil {
		return gitops.Rebased{}, err
	}
	res, err := c.Git.Rebase(ctx, mirror, branch, c.base(), expectedHead, c.Identity)
	if err != nil || res.Conflict {
		return res, err
	}
	auth, err := c.auth(ctx, repo)
	if err != nil {
		return gitops.Rebased{}, err
	}
	if err := c.Git.PushRebase(ctx, mirror, c.Remote(repo), branch, expectedHead, res.SHA, auth); err != nil {
		return gitops.Rebased{}, fmt.Errorf("conductor: publish rebase: %w", err)
	}
	return res, nil
}

// privilegedPaths are the files that project commands and deploy capabilities run, read from
// the project file at the base branch (policy.ProjectExecuted). A project without the file has none.
func (c *Conductor) privilegedPaths(ctx context.Context, mirror string) ([]string, error) {
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return nil, fmt.Errorf("conductor: %w", err)
	}
	doc, ok, err := c.Git.Show(ctx, mirror, base, ".nightshift/project.yaml")
	if err != nil || !ok {
		return nil, err
	}
	return policy.ProjectExecuted(doc)
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
