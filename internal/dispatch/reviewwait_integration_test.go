package dispatch_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// waitingRig takes a task of the given file to human_review with an outbox and a chat thread.
func waitingRig(t *testing.T, review config.Review, governance, file string) *rig {
	t.Helper()
	return reviewRig(t, review, governance, file, "C1/1700000000.000100")
}

// reviewRig takes a task of the given file to human_review with an outbox. An empty thread
// leaves the task without a chat thread, as a task added from the command line is.
func reviewRig(t *testing.T, review config.Review, governance, file, thread string) *rig {
	t.Helper()
	r := newRig(t)
	r.withGate(review, governance)
	r.c.Outbox = &dispatch.Outbox{Pool: r.c.Pool, Mask: dispatch.NewMasker("x-secret-value")}
	if thread != "" {
		if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = $2 WHERE id = $1`, r.task.ID, thread); err != nil {
			t.Fatal(err)
		}
	}
	r.reload()
	r.toReview(file)
	r.advance() // human_review
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	return r
}

func (r *rig) after(d time.Duration) {
	r.t.Helper()
	r.c.Now = func() time.Time { return time.Now().Add(d) }
}

func (r *rig) outboxCount(like string) int {
	r.t.Helper()
	var n int
	if err := r.c.Pool.QueryRow(r.t.Context(), `SELECT count(*) FROM book.outbox WHERE idempotency_key LIKE $1`, like).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

// T4, design section 11: the owner is reminded after each configured wait, once each.
func TestAReviewReminderGoesOutOncePerWait(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", RemindAfterHours: []int{4, 24}}, "solo", "feature.txt")
	r.after(3 * time.Hour)
	r.advance()
	if n := r.outboxCount("review-remind:%"); n != 0 {
		t.Fatalf("%d reminders after 3 hours, want none", n)
	}
	r.after(5 * time.Hour)
	r.advance()
	r.advance()
	if n := r.outboxCount("review-remind:%"); n != 1 {
		t.Fatalf("%d reminders after 5 hours, want one (and not again on the next tick)", n)
	}
	// The factory was down for two days: only the latest reminder is sent, and the 4-hour one is not repeated.
	r.after(49 * time.Hour)
	r.advance()
	r.advance()
	if n := r.outboxCount("review-remind:%"); n != 2 {
		t.Fatalf("%d reminders after 49 hours, want two in all", n)
	}
	if n := r.outboxCount("review-remind:%:24"); n != 1 {
		t.Fatalf("the 24-hour reminder was sent %d times", n)
	}
}

// eventCount is how many events of a kind the task has.
func (r *rig) eventCount(kind string) int64 {
	r.t.Helper()
	n, err := r.q.CountTaskEvents(r.t.Context(), bookdb.CountTaskEventsParams{TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: kind})
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

// cardThread is the thread of the one queued card whose key matches.
func (r *rig) cardThread(like string) integration.MessageRef {
	r.t.Helper()
	var payload []byte
	if err := r.c.Pool.QueryRow(r.t.Context(), `SELECT payload FROM book.outbox WHERE idempotency_key LIKE $1`, like).Scan(&payload); err != nil {
		r.t.Fatal(err)
	}
	var card integration.Card
	if err := json.Unmarshal(payload, &card); err != nil {
		r.t.Fatal(err)
	}
	return card.Thread
}

// Regression (live check of 2026-11-26): a task without a chat thread got no reminder, and the
// reminder was recorded as sent. It goes to the owner's chat, as the digest does.
func TestAReminderForATaskWithoutAThreadGoesToTheOwnersChat(t *testing.T) {
	r := reviewRig(t, config.Review{Mode: "all", RemindAfterHours: []int{4}}, "solo", "feature.txt", "")
	r.tenant.People.Owner.Chat = "U0OWNER"
	r.after(5 * time.Hour)
	r.advance()
	r.advance()
	if n := r.outboxCount("review-remind:%"); n != 1 {
		t.Fatalf("%d reminders for a task without a thread, want one", n)
	}
	if got, want := r.cardThread("review-remind:%"), (integration.MessageRef{Channel: "U0OWNER"}); got != want {
		t.Fatalf("the reminder goes to %+v, want %+v", got, want)
	}
	if n := r.eventCount("review.reminded"); n != 1 {
		t.Fatalf("%d review.reminded events, want one", n)
	}
}

// Regression: a reminder that has nowhere to go is never recorded as sent. It is noted once and
// goes out as soon as the tenant names the owner's chat.
func TestAReminderWithNowhereToGoIsNotRecordedAsSent(t *testing.T) {
	r := reviewRig(t, config.Review{Mode: "all", RemindAfterHours: []int{4}}, "solo", "feature.txt", "")
	r.after(5 * time.Hour)
	r.advance()
	r.advance()
	if sent, queued := r.eventCount("review.reminded"), r.outboxCount("review-remind:%"); sent != 0 || queued != 0 {
		t.Fatalf("recorded %d, queued %d; want nothing recorded as sent", sent, queued)
	}
	if n := r.eventCount("review.reminder_unsent"); n != 1 {
		t.Fatalf("%d review.reminder_unsent events after two ticks, want one", n)
	}
	r.tenant.People.Owner.Chat = "U0OWNER"
	r.advance()
	if sent, queued := r.eventCount("review.reminded"), r.outboxCount("review-remind:%"); sent != 1 || queued != 1 {
		t.Fatalf("recorded %d, queued %d once the owner's chat is set; want one of each", sent, queued)
	}
}

// Regression: the same holds with the chat switched off (no outbox).
func TestAReminderIsNotRecordedWhenTheChatIsOff(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", RemindAfterHours: []int{4}}, "solo", "feature.txt")
	r.c.Outbox = nil
	r.after(5 * time.Hour)
	r.advance()
	if n := r.eventCount("review.reminded"); n != 0 {
		t.Fatalf("%d review.reminded events with the chat off, want none", n)
	}
}

// The SLA notice of a task without a thread goes to the owner's chat too.
func TestTheSLANoticeForATaskWithoutAThreadGoesToTheOwnersChat(t *testing.T) {
	r := reviewRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md", "")
	r.tenant.People.Owner.Chat = "U0OWNER"
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("the task is in %s after the SLA, want ready", r.task.State)
	}
	if got, want := r.cardThread("review-sla:%"), (integration.MessageRef{Channel: "U0OWNER"}); got != want {
		t.Fatalf("the SLA notice goes to %+v, want %+v", got, want)
	}
}

func TestNoReminderWithoutTheSetting(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all"}, "solo", "feature.txt")
	r.after(100 * time.Hour)
	r.advance()
	if n := r.outboxCount("review-%"); n != 0 || r.task.State != "human_review" {
		t.Fatalf("reminders %d, state %s; want none and still waiting", n, r.task.State)
	}
}

// T4: the SLA approves a low blast-radius change that nobody reviewed, and nothing else.
func TestSLAApprovesALowChangeAfterTheWait(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.after(11 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("approved after 11 of 12 hours: %s", r.task.State)
	}
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "ready" {
		t.Fatalf("the task is in %s after the SLA, want ready", r.task.State)
	}
	if n := r.outboxCount("review-sla:%"); n != 1 {
		t.Fatalf("%d SLA notices, want one", n)
	}
}

func TestSLADoesNotApproveAHigherChange(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "feature.txt")
	r.after(48 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("a change outside the low paths was approved by the SLA: %s", r.task.State)
	}
}

func TestSLAIsOffWithoutTheKey(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all"}, "solo", "docs/guides/note.md")
	r.after(48 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA ran without sla_automerge_hours: %s", r.task.State)
	}
}

func TestSLADoesNotApproveInQuietHours(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	h := time.Now().Add(13 * time.Hour).UTC().Hour() // the window that holds the moment the SLA runs out
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: fmt.Sprintf("%02d:00-%02d:00", h, (h+1)%24)}
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved in quiet hours: %s", r.task.State)
	}
}

func TestAPersonsApprovalStillWinsOverTheSLA(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "changes_requested", CommitSHA: r.task.HeadSha.String})
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State == "ready" {
		t.Fatal("the SLA approved over a request for changes")
	}
}

// The schema forbids the key for the team profile; the code refuses it too.
func TestSLANeverApprovesForTheTeamProfile(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "team", "docs/guides/note.md")
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved a team change: %s", r.task.State)
	}
}

// Review finding: a push after the review request must not ride on the SLA of the head that was asked about.
func TestSLADoesNotApproveWhenTheHostHeadMovedAfterTheAsk(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.twin.HeadOf = nil // the twin reports its own head again, as the host does after a push
	r.twin.PushHead("example/app", 1, "later")
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved although the head on the host differs from the asked one: %s", r.task.State)
	}
}

func TestACommentFromTheOwnerStopsTheSLA(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: "commented", CommitSHA: r.task.HeadSha.String})
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved over the owner's comment: %s", r.task.State)
	}
}

func TestSLAFailsClosedOnABrokenQuietHoursSetting(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.tenant.Schedule = config.Schedule{Timezone: "UTC", QuietHours: "late"}
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved with an unreadable quiet_hours: %s", r.task.State)
	}
}

func TestSLAWithoutAReviewModeApprovesNothing(t *testing.T) {
	r := waitingRig(t, config.Review{Mode: "all", SLAAutomergeHours: 12}, "solo", "docs/guides/note.md")
	r.tenant.Review.Mode = "" // the owner switched to "hold everything" after the decision
	r.after(13 * time.Hour)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the SLA approved with no review mode: %s", r.task.State)
	}
}
