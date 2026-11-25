package dispatch_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

type cardRecorder struct {
	mu    sync.Mutex
	cards []integration.Card
}

func (r *cardRecorder) Send(_ context.Context, m dispatch.Message) error {
	var c integration.Card
	if err := json.Unmarshal(m.Payload, &c); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cards = append(r.cards, c)
	return nil
}

func (r *cardRecorder) last() integration.Card {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cards) == 0 {
		return integration.Card{}
	}
	return r.cards[len(r.cards)-1]
}

// leadRig is a rig whose task is a fresh idea in a chat thread, with the Lead stage on.
func leadRig(t *testing.T) (*rig, *cardRecorder) {
	t.Helper()
	r := newRig(t)
	rec := &cardRecorder{}
	r.c.Roles["lead"] = config.Role{Name: "lead", OutputSchema: "schemas/lead-reply.json"}
	r.c.Router.Chains["lead"] = []string{"fake"}
	r.c.Validator = config.NewValidator()
	r.c.Outbox = &dispatch.Outbox{Pool: r.c.Pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: rec}, Mask: dispatch.NewMasker()}
	r.tenant.People.Owner.Chat = "U1"
	task, err := r.q.CreateTask(t.Context(), bookdb.CreateTaskParams{
		Tenant: "example-solo", Repo: "example/app", Title: "Add CSV export", ThreadRef: pgtype.Text{String: "C1/100.1", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.task = task
	r.say("task_created", "Add CSV export")
	return r, rec
}

func (r *rig) say(kind, text string) {
	r.t.Helper()
	p, _ := json.Marshal(map[string]string{"text": text})
	if _, err := r.q.AppendEvent(r.t.Context(), bookdb.AppendEventParams{
		Tenant: r.task.Tenant, TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: kind, Actor: "chat:U1", Payload: p,
	}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) leadJob() bookdb.BookJob {
	r.t.Helper()
	jobs, err := r.q.ListTaskJobs(r.t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "lead"})
	if err != nil || len(jobs) == 0 {
		r.t.Fatalf("lead jobs: %v %v", jobs, err)
	}
	return jobs[len(jobs)-1]
}

func (r *rig) leadSays(result string) {
	r.t.Helper()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET state = 'done', exit_reason = 'done', result = $2, finished_at = now() WHERE id = $1`, r.leadJob().ID, []byte(result)); err != nil {
		r.t.Fatal(err)
	}
}

const leadSpec = `{"title":"CSV export","goal":"Export the report as CSV.","prd_path":"docs/prd/csv.md",
 "acceptance_criteria":[{"id":"AC-1","given":"a report","when":"the owner exports it","then":"a CSV file downloads"}],
 "tasks":[{"id":"T-1","title":"Add the export"}]}`

func TestLeadGrillsThenOffersASpecAndApprovalQueuesTheTask(t *testing.T) {
	r, rec := leadRig(t)
	ctx := t.Context()

	r.advance() // idea -> grilling, a Lead job is queued
	if r.task.State != "grilling" {
		t.Fatalf("the task is in %s", r.task.State)
	}
	if !strings.Contains(string(r.leadJob().Payload), "Add CSV export") {
		t.Fatalf("the prompt lacks the owner's message: %s", r.leadJob().Payload)
	}
	r.advance() // still running
	r.leadSays(`{"type":"question","text":"Which columns?","options":[{"label":"All"},{"label":"Visible only"}],"recommended":0}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if c := rec.last(); !strings.Contains(c.Text, "Which columns?") || !strings.Contains(c.Text, "(recommended)") || c.Thread.ID != "100.1" || len(c.Buttons) != 0 {
		t.Fatalf("question card: %+v", c)
	}
	r.advance() // nothing new from the owner: no second job
	if jobs, _ := r.q.ListTaskJobs(ctx, bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "lead"}); len(jobs) != 1 {
		t.Fatalf("%d lead jobs without a new message", len(jobs))
	}

	r.say("owner_message", "All columns")
	r.advance() // a second job carrying the whole thread
	if p := string(r.leadJob().Payload); !strings.Contains(p, "Which columns?") || !strings.Contains(p, "All columns") {
		t.Fatalf("second prompt: %s", p)
	}
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"spec":` + leadSpec + `}`)
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("after the spec the task is in %s", r.task.State)
	}
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	card := rec.last()
	if len(card.Buttons) != 3 || !strings.Contains(card.Text, "AC-1") || card.Version != int(r.task.Version) {
		t.Fatalf("spec card: %+v (task version %d)", card, r.task.Version)
	}

	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	click := func(action string, version int) {
		t.Helper()
		if err := inbox.Handle(ctx, r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: action, TaskID: itoa(r.task.ID), CardVersion: version}); err != nil {
			t.Fatal(err)
		}
		r.reload()
	}
	click("approve", int(r.task.Version)-1) // a stale card
	if r.task.State != "spec" {
		t.Fatalf("a stale click moved the task to %s", r.task.State)
	}
	click("approve", int(r.task.Version))
	if r.task.State != string(flow.Queued) {
		t.Fatalf("approval left the task in %s", r.task.State)
	}

	r.advance() // queued -> building: Builder gets the spec
	if p := string(r.lastJob().Payload); !strings.Contains(p, "The owner approved this spec") || !strings.Contains(p, "a CSV file downloads") {
		t.Fatalf("builder prompt: %s", p)
	}
	// The pull request is named by the approved spec, not by the raw chat message.
	r.finish(r.patchFor("export.txt", "done\n"))
	r.advance()
	if pr, ok := r.twin.PR("example/app", 1); !ok || pr.Title != "CSV export" {
		t.Fatalf("pull request title: %+v %v", pr, ok)
	}
}

func TestLeadIsToldToAnswerInTheOwnersLanguage(t *testing.T) {
	r, _ := leadRig(t)
	r.say("owner_message", "Добавь экспорт в CSV")
	r.advance()
	if p := string(r.leadJob().Payload); !strings.Contains(p, "in Russian") {
		t.Fatalf("the prompt does not name the language: %s", p)
	}
}

func TestLeadReplyThatBreaksTheRulesIsNotShown(t *testing.T) {
	r, rec := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"decision_needed","text":"Pick one."}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := rec.last(); strings.Contains(c.Text, "Pick one") || !strings.Contains(c.Text, "retry") {
		t.Fatalf("the owner saw %q", c.Text)
	}
	if r.task.State != "grilling" {
		t.Fatalf("an invalid reply left the task in %s", r.task.State)
	}
	r.say("owner_message", "go on")
	r.advance()
	if p := r.leadJob(); p.State != "queued" {
		t.Fatalf("the retry job is %s", p.State)
	}
}

func TestEditSendsTheSpecBackToGrilling(t *testing.T) {
	r, _ := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"blocker_or_summary","text":"Done.","spec_ready":true,"spec":` + leadSpec + `}`)
	r.advance()
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(t.Context(), r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "edit", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	if r.task.State != "grilling" {
		t.Fatalf("edit left the task in %s", r.task.State)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
