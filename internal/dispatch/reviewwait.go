package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/policy"
)

const (
	eventReviewReminded = "review.reminded"
	eventSLAApproved    = "review.sla_approved"
)

// reviewWait runs while a pull request waits for a person (design section 11): it reminds the owner
// after the tenant's `review.remind_after_hours`, and, when the tenant switched on
// `review.sla_automerge_hours`, approves a low blast-radius change that nobody looked at in time.
func (c *Conductor) reviewWait(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref integration.PRRef) error {
	if len(t.Review.RemindAfterHours) == 0 && t.Review.SLAAutomergeHours <= 0 {
		return nil
	}
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	asked, ok := askedAt(events, ref.HeadSHA)
	if !ok {
		return nil
	}
	waited := c.now().Sub(asked)
	if t.Review.SLAAutomergeHours > 0 && waited >= time.Duration(t.Review.SLAAutomergeHours)*time.Hour {
		if done, err := c.slaApprove(ctx, t, task, ref, events, waited); done || err != nil {
			return err
		}
	}
	return c.remind(ctx, t, task, ref.HeadSHA, events, waited)
}

// askedAt is when the review of this head was requested.
func askedAt(events []bookdb.BookEvent, head string) (time.Time, bool) {
	for _, e := range slices.Backward(events) {
		if e.Kind == eventReviewAsked && strings.Contains(string(e.Payload), head) && e.CreatedAt.Valid {
			return e.CreatedAt.Time, true
		}
	}
	return time.Time{}, false
}

// noteOf is the note of an event written by appendEvent.
func noteOf(e bookdb.BookEvent) string {
	var p struct {
		Note string `json:"note"`
	}
	_ = json.Unmarshal(e.Payload, &p)
	return p.Note
}

// remind tells the owner that a review waits. When several reminders came due together (the
// factory was down), only the latest is sent; all are recorded, so none repeats. A reminder is a
// decision card, so quiet hours hold it until morning.
func (c *Conductor) remind(ctx context.Context, t config.Tenant, task bookdb.BookTask, head string, events []bookdb.BookEvent, waited time.Duration) error {
	var due []int
	for _, h := range t.Review.RemindAfterHours {
		if h > 0 && waited >= time.Duration(h)*time.Hour &&
			!slices.ContainsFunc(events, func(e bookdb.BookEvent) bool {
				return e.Kind == eventReviewReminded && noteOf(e) == head+":"+strconv.Itoa(h)
			}) {
			due = append(due, h)
		}
	}
	if len(due) == 0 {
		return nil
	}
	slices.Sort(due)
	latest := due[len(due)-1]
	if ref, err := threadOf(task); err == nil && c.Outbox != nil {
		text := fmt.Sprintf("Task #%d has waited %d hours for your review of pull request #%d. Approve it, ask for changes, or close it.", task.ID, latest, prNumber(task))
		key := fmt.Sprintf("review-remind:%d:%s:%d", task.ID, head, latest)
		if err := c.postCard(ctx, bookdb.New(c.Pool), task, integration.CardDecisionNeeded, text, nil, ref, key); err != nil {
			return err
		}
	}
	for _, h := range due {
		c.appendEvent(ctx, task, eventReviewReminded, head+":"+strconv.Itoa(h))
	}
	return nil
}

// slaApprove approves the pull request when the SLA has run out. It never runs for the team
// profile, in quiet hours (no release at night, not even by SLA), or for a change above `low`:
// the level comes from the policy decision of this task, so a Verifier that raised the blast radius
// leaves no `low` decision and the change keeps waiting for a person.
func (c *Conductor) slaApprove(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref integration.PRRef, events []bookdb.BookEvent, waited time.Duration) (bool, error) {
	if c.profile(t) == flow.Team || c.quiet(ctx, t) && !t.Schedule.ReleasesInQuietHours {
		return false, nil
	}
	if raised := slices.ContainsFunc(events, func(e bookdb.BookEvent) bool { return e.Kind == eventRaised }); raised {
		return false, nil
	}
	level := ""
	for _, e := range slices.Backward(events) {
		if e.Kind == eventReviewDecided {
			level, _, _ = strings.Cut(noteOf(e), " ")
			break
		}
	}
	if level != string(policy.Low) {
		return false, nil
	}
	if err := c.approveReview(ctx, t, task, ref, fmt.Sprintf("approved by the SLA after %d hours without a review", t.Review.SLAAutomergeHours)); err != nil {
		return false, err
	}
	c.appendEvent(ctx, task, eventSLAApproved, fmt.Sprintf("%s: no review in %d hours", ref.HeadSHA, int(waited.Hours())))
	if tref, err := threadOf(task); err == nil && c.Outbox != nil {
		text := fmt.Sprintf("Task #%d: pull request #%d was low risk and nobody reviewed it in %d hours, so it goes to the merge train (SLA auto-merge). Revert it from the pull request if you disagree.", task.ID, prNumber(task), t.Review.SLAAutomergeHours)
		if err := c.postCard(ctx, bookdb.New(c.Pool), task, integration.CardBlocker, text, nil, tref, fmt.Sprintf("review-sla:%d:%s", task.ID, ref.HeadSHA)); err != nil {
			c.log().WarnContext(ctx, "SLA notice not queued", "task", task.ID, "error", err)
		}
	}
	return true, nil
}
