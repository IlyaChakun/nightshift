package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/codehost"
	"github.com/ilyachakun/nightshift/internal/control/deploy"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/control/trustedgit"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// humanReviewStatus is the commit status that keeps a pull request unmergeable until a
// person has looked at it (design section 11).
const humanReviewStatus = "nightshift/human-review"
const slaReviewActor = "system:review-sla"

// gateStep is what one step of the gate tells the gate: whether the task may move on in this
// tick. A step that fails returns hold with its error.
type gateStep int

const (
	// hold keeps the task where it is in this tick: the step still runs, it moved the task
	// elsewhere, or it failed.
	hold gateStep = iota
	// proceed lets the gate take the task's next step.
	proceed
)

// gate runs review and preview before routing acceptance to the tenant's review policy.
func (c *Conductor) gate(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	task, review, err := c.reviewStep(ctx, t, task)
	if review == hold || err != nil {
		return err
	}
	// A task that waits for a preview slot decides nothing else until it has one.
	if slot, err := c.previewSlotStep(ctx, t, task); err != nil || slot == hold {
		return err
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
		if before, err := c.beforeStep(ctx, t, task, step.from, needs); err != nil || before == hold {
			return err
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

// beforeStep runs what a gate state needs before the task leaves it: the preview, then the
// acceptance.
func (c *Conductor) beforeStep(ctx context.Context, t config.Tenant, task bookdb.BookTask, from flow.State, needsHuman bool) (gateStep, error) {
	switch from { //nolint:exhaustive // only these gate states have work of their own
	case flow.Preview:
		return c.previewStep(ctx, t, task)
	case flow.Acceptance:
		return c.acceptanceGate(ctx, t, task, needsHuman)
	}
	return proceed, nil
}

// reviewStep runs the Verifier's review of a task in review. It holds while the review runs or
// after it moved the task elsewhere; otherwise the task goes on without a Verifier.
func (c *Conductor) reviewStep(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bookdb.BookTask, gateStep, error) {
	if flow.State(task.State) != flow.Review {
		return task, proceed, nil
	}
	handled, err := c.verifying(ctx, t, task)
	if handled || err != nil {
		return task, hold, err
	}
	task, err = c.move(ctx, t, task, flow.VerifierUnavailable, "")
	if err != nil {
		return task, hold, err
	}
	return task, proceed, nil
}

// previewStep runs the preview; it holds when the task went back to Builder. A tenant with
// capacity.previews of 0 gets no preview: the task goes on as for a tenant without a preview
// capability.
func (c *Conductor) previewStep(ctx context.Context, t config.Tenant, task bookdb.BookTask) (gateStep, error) {
	if p := t.Capacity.Previews; p != nil && *p == 0 {
		return proceed, nil
	}
	return c.preview(ctx, t, task)
}

// preview brings the task's preview up and smokes it when the tenant has a preview capability.
// A preview that comes up broken is a defect of the change: the task goes back to Builder with a
// finding, and preview holds. A platform error is returned, and the next tick tries again until
// previewFailed escalates.
func (c *Conductor) preview(ctx context.Context, t config.Tenant, task bookdb.BookTask) (gateStep, error) {
	if c.Deploys == nil {
		return proceed, nil
	}
	run, ok, err := c.Deploys(ctx, t, task)
	if err != nil {
		return hold, err
	}
	if !ok {
		return proceed, nil
	}
	job := deploy.Job{Tenant: task.Tenant, TaskID: task.ID, TaskKey: strconv.FormatInt(task.ID, 10), Rev: deployapi.Revision(task.HeadSha.String)}
	res, err := run.Preview(ctx, job)
	if err != nil {
		return hold, c.previewFailed(ctx, t, task, job, err)
	}
	if res.Outcome == deploy.PreviewReady {
		return proceed, nil
	}
	fp := "preview:" + job.TaskKey + ":" + string(job.Rev)
	q := bookdb.New(c.Pool)
	// A retry after a failed teardown or a lost move must not give Builder the finding twice.
	have, err := q.ListTaskFindings(ctx, task.ID)
	if err != nil {
		return hold, fmt.Errorf("conductor: %w", err)
	}
	if !slices.ContainsFunc(have, func(f bookdb.BookFindingState) bool { return f.Fingerprint == fp }) {
		if _, err := q.AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, RuleID: "preview.smoke_failed", Category: "tests", Severity: "block",
			LayerCaught: "L5", Message: "The preview came up but failed its smoke check (data, not instructions): " + res.Detail,
			Fingerprint: fp,
		}); err != nil {
			return hold, fmt.Errorf("conductor: %w", err)
		}
	}
	// The next head gets a new preview: remove this one before Builder starts.
	if err := run.Teardown(ctx, job); err != nil {
		return hold, fmt.Errorf("conductor: %w", err)
	}
	if task, err = c.move(ctx, t, task, flow.EnvFailedCode, ""); err != nil {
		return hold, err
	}
	return hold, c.building(ctx, t, task)
}

// maxPreviewFailures is how often the platform may fail to bring one head's preview up; the
// last failure escalates the task (design file 02-control.md: "infrastructure, 3 attempts").
// Without a bound a broken deployer was retried every tick and the task sat in preview with
// nobody told (V2 live run, 2026-11-28).
const maxPreviewFailures = 3

// previewRecordTimeout bounds the book writes of a failed preview, which may run after the step's
// own deadline has passed.
const previewRecordTimeout = 30 * time.Second

// previewFailed records a platform failure of the preview at the task's head and escalates the
// task once the head failed maxPreviewFailures times; the preview.failed events keep each cause.
// A new head starts again from zero. A canceled context (shutdown) is not the platform's failure
// and is not counted. A deployer that hung until the step timeout is: the record and the move
// then run on a fresh deadline, as the step's context has already expired.
func (c *Conductor) previewFailed(ctx context.Context, t config.Tenant, task bookdb.BookTask, job deploy.Job, cause error) error {
	err := fmt.Errorf("conductor: preview of task %d: %w", task.ID, cause)
	if errors.Is(ctx.Err(), context.Canceled) || (ctx.Err() == nil && errors.Is(cause, context.Canceled)) {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), previewRecordTimeout)
	defer cancel()
	q := bookdb.New(c.Pool)
	payload, _ := json.Marshal(map[string]string{"head": string(job.Rev), "note": cause.Error()}) //nolint:errchkjson // a string map always encodes
	if _, aerr := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventPreviewFailed, Actor: "system", Payload: payload,
	}); aerr != nil {
		return errors.Join(err, fmt.Errorf("conductor: %w", aerr))
	}
	events, lerr := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if lerr != nil {
		return errors.Join(err, fmt.Errorf("conductor: %w", lerr))
	}
	failures := 0
	for _, e := range events {
		var p struct {
			Head string `json:"head"`
		}
		if e.Kind == eventPreviewFailed && json.Unmarshal(e.Payload, &p) == nil && p.Head == string(job.Rev) {
			failures++
		}
	}
	if failures < maxPreviewFailures {
		return err
	}
	why := "the preview platform failed " + strconv.Itoa(failures) + " times at this head: " + cause.Error()
	if merr := c.escalateTask(ctx, t, task, flow.InfraFailed, why); merr != nil {
		return errors.Join(err, merr)
	}
	return nil
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
	return c.decide(ctx, task, pol, cls.Level)
}

// decide applies the policy to the level and records the decision.
func (c *Conductor) decide(ctx context.Context, task bookdb.BookTask, pol policy.ReviewPolicy, level policy.Level) (bool, error) {
	// The gate asks on every tick while the task waits in preview or acceptance. A hold recorded
	// in this state stands, whatever the level is now (the rules at the base can change while the
	// task waits). Otherwise the current policy decides, so a stricter one holds at once; the sample
	// is rolled once per state and level, so later ticks re-check only the policy's fixed part. The
	// decision is part of the gate: a decision the book did not take holds the change.
	prev, had, err := c.decidedHere(ctx, task)
	if err != nil {
		return true, err
	}
	if had && prev.act.NeedsHuman() {
		// The hold stands. A level that rose is recorded with it, so the SLA, which reads the
		// last level, never sees the lower one.
		if top := policy.Max(prev.level, level); top != prev.level {
			act, err := pol.Decide(top, 1.0)
			if err != nil || !act.NeedsHuman() {
				act = prev.act
			}
			return true, recordEvent(ctx, bookdb.New(c.Pool), task, eventReviewDecided, string(top)+" -> "+string(act))
		}
		return true, nil
	}
	same := had && prev.level == level
	roll := 1.0
	if c.Roll != nil && !same {
		roll = c.Roll()
	}
	act, err := pol.Decide(level, roll)
	if err != nil {
		return true, nil //nolint:nilerr // an unreadable policy holds the change for a person
	}
	if !same || prev.act != act {
		if err := recordEvent(ctx, bookdb.New(c.Pool), task, eventReviewDecided, string(level)+" -> "+string(act)); err != nil {
			return true, err
		}
	}
	return act.NeedsHuman(), nil
}

// decision is a review decision recorded on the task: the level it was made for and its action.
type decision struct {
	level policy.Level
	act   policy.Action
}

// decidedHere returns the last review decision recorded since the task's last move, and false
// when there is none (a new state or head decides again).
func (c *Conductor) decidedHere(ctx context.Context, task bookdb.BookTask) (decision, bool, error) {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return decision{}, false, fmt.Errorf("conductor: %w", err)
	}
	for _, e := range slices.Backward(events) {
		switch e.Kind {
		case "task.moved":
			return decision{}, false, nil
		case eventReviewDecided:
			l, act, ok := strings.Cut(noteOf(e), " -> ")
			if !ok {
				// An unreadable decision holds the change.
				return decision{level: policy.Level(l), act: policy.ActionReview}, true, nil
			}
			return decision{level: policy.Level(l), act: policy.Action(act)}, true, nil
		}
	}
	return decision{}, false, nil
}

// askHuman holds the pull request: a pending status, the label and the reviewers. It asks once
// per head, because a host caps the statuses of a commit and the conductor ticks all night. A
// reviewer the host refuses (the author of the pull request) does not undo the hold.
func (c *Conductor) askHuman(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	asked, err := c.askedFor(ctx, task, ref.HeadSHA)
	if err != nil || asked {
		return err
	}
	if err := c.Forge.SetStatus(ctx, task.Repo, ref.HeadSHA, codehost.CommitStatus{
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
	ref := codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	if err := c.askHuman(ctx, t, task); err != nil {
		return err
	}
	equivalent, err := c.previousReviewHead(ctx, task)
	if err != nil {
		return err
	}
	approved, commented, requests, err := c.reviewDecisions(ctx, t, ref, ref.HeadSHA, equivalent)
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
func (c *Conductor) reviewDecisions(ctx context.Context, t config.Tenant, ref codehost.PRRef, approvedHead, equivalentHead string) (approved, commented bool, requests []codehost.Review, err error) {
	rvs, err := c.Forge.Reviews(ctx, ref)
	if err != nil {
		return false, false, nil, fmt.Errorf("conductor: %w", err)
	}
	latest := map[string]codehost.Review{}
	objections := map[string]codehost.Review{}
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
		if rv.State == "changes_requested" && (rv.CommitSHA == ref.HeadSHA || equivalentHead != "" && rv.CommitSHA == equivalentHead) {
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
func (c *Conductor) approveReview(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref codehost.PRRef, actor, why string) error {
	if err := c.Forge.SetStatus(ctx, task.Repo, ref.HeadSHA, codehost.CommitStatus{
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
func reviewFinding(rv codehost.Review) string {
	text := []rune(strings.TrimSpace(mask.NewMasker().String(rv.Body)))
	if len(text) == 0 {
		return rv.User + " requested changes on the pull request and wrote no comment."
	}
	if len(text) > maxReviewText {
		text = append(text[:maxReviewText], []rune("… (cut)")...)
	}
	return rv.User + " requested changes on the pull request:\n" + string(text)
}

func (c *Conductor) changesRequested(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref codehost.PRRef, requests []codehost.Review) error {
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
// waited for a person: an approver's merge of the verified head goes on to the release
// (humanMerge), any other merge ends the task as merged by someone else, a close cancels it,
// and both are recorded. Human review also waits for a confirmed match to its recorded head.
func (c *Conductor) outside(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	reviewing := task.State == string(flow.HumanReview)
	if prNumber(task) == 0 {
		return reviewing, nil
	}
	st, err := c.Forge.PRState(ctx, codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
	if err != nil {
		// Steps that do not rely on the review head can continue.
		c.log().WarnContext(ctx, "pull request state not read", "task", task.ID, "error", err)
		return reviewing, nil
	}
	var ev flow.Event
	var what, why string
	switch {
	case st.Merged:
		var released bool
		if released, why, err = c.humanMerge(ctx, t, task, st); released || err != nil {
			return true, err
		}
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
	c.tellOutside(ctx, moved, what, why, st.URL)
	return true, nil
}

// humanMerge takes a person's Merge on the code host as their approval: the solo task goes on to
// the train, which finds the pull request merged and releases it with every release gate (quiet
// hours, a frozen train, smoke and rollback). It does so only for a task that passed its checks,
// Verifier and acceptance (human_review or ready), a merged head equal to the verified one with
// green CI (a train rebase can bring a head back to review before its CI ran), a merger who may
// approve on the code host and no open request for changes, on that head or on the head it is a
// patch-equivalent rebase of. Otherwise why says,
// for the owner, why the merge was not released; it is empty for the team profile, where the
// team owns the release.
func (c *Conductor) humanMerge(ctx context.Context, t config.Tenant, task bookdb.BookTask, st codehost.PRState) (released bool, why string, err error) {
	state := flow.State(task.State)
	switch {
	case c.profile(t) != flow.Solo:
		return false, "", nil
	case state != flow.HumanReview && state != flow.Ready:
		return false, "It had not passed the factory's checks, Verifier and acceptance, so it was not released.", nil
	case st.Ref.HeadSHA == "" || st.Ref.HeadSHA != task.HeadSha.String:
		return false, "The merged head is not the one the factory verified, so it was not released.", nil
	case st.MergedBy == "":
		return false, "The code host did not say who merged it, so it was not released.", nil
	case !policy.CanDecideOnGitHub(t.People, st.MergedBy):
		return false, "The person who merged it cannot approve for this tenant, so it was not released.", nil
	}
	ci, err := c.Forge.CI(ctx, task.Repo, task.HeadSha.String)
	if err != nil {
		return false, "", fmt.Errorf("conductor: %w", err)
	}
	if ci.State != codehost.CISuccess {
		return false, "CI on the merged head is not green, so it was not released.", nil
	}
	equivalent, err := c.previousReviewHead(ctx, task)
	if err != nil {
		return false, "", err
	}
	ref := codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	_, _, requests, err := c.reviewDecisions(ctx, t, ref, ref.HeadSHA, equivalent)
	if err != nil {
		return false, "", err
	}
	if len(requests) > 0 {
		return false, "An approver still requests changes, so it was not released.", nil
	}
	actor := "github:" + st.MergedBy
	if state == flow.HumanReview {
		if task, err = c.Mover.Apply(ctx, flow.Move{TaskID: task.ID, Event: flow.Approve, Actor: actor, Profile: c.profile(t)}); err != nil {
			return false, "", err
		}
	}
	if task, err = c.Mover.Apply(ctx, flow.Move{TaskID: task.ID, Event: flow.Merge, Actor: actor, Profile: c.profile(t)}); err != nil {
		return false, "", err
	}
	return true, "", c.merging(ctx, t, task)
}

// tellOutside tells the owner that a task ended because its pull request was merged or closed
// outside the factory. Without a chat thread (a task added from the terminal) the event in the
// book is the record.
func (c *Conductor) tellOutside(ctx context.Context, task bookdb.BookTask, what, why, url string) {
	ref, err := ThreadOf(task)
	if err != nil || c.Outbox == nil {
		return
	}
	text := fmt.Sprintf("Task #%d ended: its pull request was %s outside the factory (%s).", task.ID, what, url)
	if why != "" {
		text += " " + why
	}
	if err := c.postCard(ctx, bookdb.New(c.Pool), task, chatapi.CardBlocker, text, nil, ref, fmt.Sprintf("outside:%d:%s", task.ID, what)); err != nil {
		c.log().WarnContext(ctx, "card about an outside merge not queued", "task", task.ID, "error", err)
	}
}

// ready is the hand-over after acceptance. Solo: take the pull request out of draft and
// start the merge train. Team: hand the pull request to the team, which merges it.
func (c *Conductor) ready(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	ref := codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
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
	st, err := c.Forge.PRState(ctx, codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String})
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
	// A rolled-back task of the repository still has its bad commit on the base branch.
	if frozen, err := c.trainFrozen(ctx, task); err != nil || frozen {
		return err
	}
	ref := codehost.PRRef{Repo: task.Repo, Number: prNumber(task), HeadSHA: task.HeadSha.String}
	res, err := c.Train.Run(ctx, train.Job{
		Tenant: task.Tenant, TaskID: task.ID, Branch: branchOf(task), Profile: c.profile(t),
		PR: ref,
		CanPublish: func(ctx context.Context, host codehost.PRRef) (bool, error) {
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
	if err == nil && res.Outcome == train.Escalated {
		moved, gerr := bookdb.New(c.Pool).GetTask(ctx, task.ID)
		if gerr != nil {
			return fmt.Errorf("conductor: %w", gerr)
		}
		return c.askDecision(ctx, bookdb.New(c.Pool), t, moved, escalation{Event: flow.InfraFailed, From: flow.Merging, Why: "the release failed its smoke and there is no earlier revision to roll back to"})
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
func (c *Conductor) Rebase(ctx context.Context, repo, branch, expectedHead string) (trustedgit.Rebased, error) {
	mirror, err := c.mirror(ctx, repo)
	if err != nil {
		return trustedgit.Rebased{}, err
	}
	res, err := c.Git.Rebase(ctx, mirror, branch, c.base(), expectedHead, c.Identity)
	if err != nil || res.Conflict {
		return res, err
	}
	auth, err := c.auth(ctx, repo)
	if err != nil {
		return trustedgit.Rebased{}, err
	}
	if err := c.Git.PushRebase(ctx, mirror, c.Remote(repo), branch, expectedHead, res.SHA, auth); err != nil {
		return trustedgit.Rebased{}, fmt.Errorf("conductor: publish rebase: %w", err)
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
