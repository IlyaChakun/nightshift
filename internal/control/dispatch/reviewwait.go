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

	"github.com/ilyachakun/nightshift/internal/assurance/policy"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/codehost"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

const (
	eventReviewReminded = "review.reminded"
	// eventReminderUnsent records, once per reminder, that it had nowhere to go; it does not
	// stop the reminder, which goes out on the tick after the tenant gets a place for it.
	eventReminderUnsent = "review.reminder_unsent"
	eventSLAApproved    = "review.sla_approved"
)

// reviewWait runs while a pull request waits for a person (design section 11): it reminds the owner
// after the tenant's `review.remind_after_hours`, and, when the tenant switched on
// `review.sla_automerge_hours`, approves a low blast-radius change that nobody looked at in time.
func (c *Conductor) reviewWait(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref codehost.PRRef, commented bool) error {
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
		if done, err := c.slaApprove(ctx, t, task, ref, events, waited, commented); done || err != nil {
			return err
		}
	}
	return c.remind(ctx, t, task, ref.HeadSHA, events, waited)
}

// askedAt is when the review of this head was requested.
func askedAt(events []bookdb.BookEvent, head string) (time.Time, bool) {
	for _, e := range slices.Backward(events) {
		if e.Kind == "task.moved" && readMove(e).Event == flow.ReviewRequired {
			return time.Time{}, false
		}
		if e.Kind == eventReviewAsked && noteOf(e) == head && e.CreatedAt.Valid {
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

// ownerThread is where a message about a waiting review goes: the task's chat thread, or, for a
// task that has none (one added from the command line, or made from a factory health finding),
// the owner's chat, where the morning digest goes too.
func ownerThread(t config.Tenant, task bookdb.BookTask) (chatapi.MessageRef, bool) {
	if ref, err := threadOf(task); err == nil {
		return ref, true
	}
	if t.People.Owner.Chat != "" {
		return chatapi.MessageRef{Channel: t.People.Owner.Chat}, true
	}
	return chatapi.MessageRef{}, false
}

// remind tells the owner that a review waits. When several reminders came due together (the
// factory was down), only the latest is sent; all are recorded, so none repeats. A reminder is
// recorded only once it is queued for the chat: one that has nowhere to go is logged, noted on
// the task once, and tried again on every tick. A reminder is a decision card, so quiet hours
// hold it until morning.
func (c *Conductor) remind(ctx context.Context, t config.Tenant, task bookdb.BookTask, head string, events []bookdb.BookEvent, waited time.Duration) error {
	noted := func(kind string, h int) bool {
		return slices.ContainsFunc(events, func(e bookdb.BookEvent) bool {
			return e.Kind == kind && noteOf(e) == head+":"+strconv.Itoa(h)
		})
	}
	var due []int
	for _, h := range t.Review.RemindAfterHours {
		if h > 0 && waited >= time.Duration(h)*time.Hour && !noted(eventReviewReminded, h) {
			due = append(due, h)
		}
	}
	if len(due) == 0 {
		return nil
	}
	slices.Sort(due)
	latest := due[len(due)-1]
	ref, ok := ownerThread(t, task)
	if !ok || c.Outbox == nil {
		if !noted(eventReminderUnsent, latest) {
			c.log().WarnContext(ctx, "review reminder not sent: the task has no chat thread and the tenant no people.owner.chat, or the chat is off",
				"tenant", t.Name, "task", task.ID, "hours", latest)
			c.appendEvent(ctx, task, eventReminderUnsent, head+":"+strconv.Itoa(latest))
		}
		return nil
	}
	text := fmt.Sprintf("Task #%d has waited %d hours for your review of pull request #%d. Approve it, ask for changes, or close it.", task.ID, latest, prNumber(task))
	key := fmt.Sprintf("review-remind:%d:%s:%d", task.ID, head, latest)
	if err := c.postCard(ctx, bookdb.New(c.Pool), task, chatapi.CardDecisionNeeded, text, nil, ref, key); err != nil {
		c.log().WarnContext(ctx, "review reminder not queued", "tenant", t.Name, "task", task.ID, "hours", latest, "error", err)
		return err
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
func (c *Conductor) slaApprove(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref codehost.PRRef, events []bookdb.BookEvent, waited time.Duration, commented bool) (bool, error) {
	if c.profile(t) == flow.Team || t.Review.Mode == "" || commented {
		return false, nil
	}
	// A quiet-hours setting that cannot be read fails closed: nobody approves at night by mistake.
	_, quiet, qerr := t.Schedule.QuietUntil(c.now())
	if qerr != nil {
		c.log().WarnContext(ctx, "SLA waits: quiet hours unreadable", "tenant", t.Name, "error", qerr)
		return false, nil
	}
	if quiet && !t.Schedule.ReleasesInQuietHours {
		return false, nil
	}
	// The head on the code host must be the one that was asked about: a push after the review
	// request would otherwise ship unreviewed by the SLA.
	st, err := c.Forge.PRState(ctx, ref)
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	if st.State != "open" || st.Merged || st.Ref.HeadSHA != ref.HeadSHA {
		return false, nil
	}
	if raised := slices.ContainsFunc(events, func(e bookdb.BookEvent) bool { return e.Kind == eventRaised }); raised {
		return false, nil
	}
	level := decidedLevel(events, ref.HeadSHA)
	if level != string(policy.Low) {
		return false, nil
	}
	if ok, err := c.hasPassingReview(ctx, task); err != nil || !ok {
		return false, err
	}
	if err := c.approveReview(ctx, t, task, ref, slaReviewActor, fmt.Sprintf("approved by the SLA after %d hours without a review", t.Review.SLAAutomergeHours)); err != nil {
		return false, err
	}
	c.appendEvent(ctx, task, eventSLAApproved, fmt.Sprintf("%s: no review in %d hours", ref.HeadSHA, int(waited.Hours())))
	tref, ok := ownerThread(t, task)
	if !ok || c.Outbox == nil {
		c.log().WarnContext(ctx, "SLA notice not sent: the task has no chat thread and the tenant no people.owner.chat, or the chat is off", "tenant", t.Name, "task", task.ID)
		return true, nil
	}
	text := fmt.Sprintf("Task #%d: pull request #%d was low risk and nobody reviewed it in %d hours, so it goes to the merge train (SLA auto-merge). Revert it from the pull request if you disagree.", task.ID, prNumber(task), t.Review.SLAAutomergeHours)
	if err := c.postCard(ctx, bookdb.New(c.Pool), task, chatapi.CardBlocker, text, nil, tref, fmt.Sprintf("review-sla:%d:%s", task.ID, ref.HeadSHA)); err != nil {
		c.log().WarnContext(ctx, "SLA notice not queued", "task", task.ID, "error", err)
	}
	return true, nil
}

// decidedLevel is the blast radius level the policy gave the head that waits: the last decision
// that no later review request for another head separates from this head's request. A change of
// settings that skipped the decision (an empty mode) leaves no level, so a stale `low` of an older
// head never counts.
func decidedLevel(events []bookdb.BookEvent, head string) string {
	asked := -1
	for i, e := range events {
		if e.Kind == eventReviewAsked && strings.Contains(string(e.Payload), head) {
			asked = i
		}
	}
	for i := asked - 1; i >= 0; i-- {
		switch events[i].Kind {
		case eventReviewDecided:
			level, _, _ := strings.Cut(noteOf(events[i]), " ")
			return level
		case eventReviewAsked:
			return ""
		case "task.moved":
			if readMove(events[i]).Event == flow.ReviewRequired {
				return ""
			}
		}
	}
	return ""
}
