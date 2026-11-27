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
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
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
	r.leadSays(`{"type":"question","understood":"The owner wants the change described in the task.","text":"Which columns?","options":[{"label":"All"},{"label":"Visible only"}],"recommended":0,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if c := rec.last(); !strings.Contains(c.Text, "I understood: ") || !strings.Contains(c.Text, "Which columns?") || !strings.Contains(c.Text, "(recommended)") || c.Thread.ID != "100.1" || len(c.Buttons) != 0 {
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
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`)
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

// TestLeadReplyThatBreaksTheRulesGoesBackOnce: the first broken reply is sent back to Lead with
// the reason and the owner sees nothing; a second one to the same message tells the owner.
func TestLeadReplyThatBreaksTheRulesGoesBackOnce(t *testing.T) {
	r, rec := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one."}`)
	r.advance() // sends it back
	r.advance() // queues the correction
	if j := r.leadJob(); j.State != "queued" || !strings.Contains(string(j.Payload), "needs two or three options") {
		t.Fatalf("the correction job is %s: %s", j.State, j.Payload)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(rec.cards) != 0 {
		t.Fatalf("the owner saw %d card(s) after the first broken reply", len(rec.cards))
	}
	r.leadSays(`{"type":"decision_needed","understood":"The owner wants the change described in the task.","text":"Pick one again."}`)
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
	r.leadSays(`{"type":"blocker_or_summary","text":"Done.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`)
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

// TestLeadRetriesAfterALimit: a Lead job that hit a limit is queued again without a new
// message from the owner, and the owner hears nothing about it.
func TestLeadRetriesAfterALimit(t *testing.T) {
	r, rec := leadRig(t)
	r.advance() // queues the first job
	r.limitJob(r.leadJob().ID)
	r.advance() // closes it
	r.advance() // queues the next one
	if j := r.leadJob(); j.State != "queued" {
		t.Fatalf("after the limit the newest Lead job is %s", j.State)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(rec.cards) != 0 {
		t.Fatalf("a limit sent the owner %d card(s)", len(rec.cards))
	}
}

// Done criterion of phase 5: on three prepared duplicates Lead opens no task and gives a link.
// The conductor puts the earlier tasks that match into Lead's prompt; Lead (a fake here) answers
// with `duplicate_of`; the owner gets the reference and the task closes without a spec.
func TestLeadAnswersAThreeDuplicatesWithALinkAndOpensNoTask(t *testing.T) {
	// The rig's task asks "Add CSV export"; each earlier task is the same request in other words.
	cases := []string{"Export the quarterly report as a CSV file", "CSV export button on the reports page", "Let users export their data to CSV"}
	for _, earlier := range cases {
		tc := struct{ earlier string }{earlier}
		t.Run(tc.earlier, func(t *testing.T) {
			r, rec := leadRig(t)
			ctx := t.Context()
			old := r.releasedTask(tc.earlier)
			r.advance()
			if p := string(r.leadJob().Payload); !strings.Contains(p, "task "+itoa(old.ID)+", released") {
				t.Fatalf("the prompt does not list the earlier task: %s", p)
			}
			r.leadSays(`{"type":"blocker_or_summary","text":"This repeats an earlier request.","duplicate_of":` + itoa(old.ID) + `,"basis":{"checked":["the book search"],"confidence":"high"}}`)
			r.advance()
			if _, err := r.c.Outbox.Flush(ctx); err != nil {
				t.Fatal(err)
			}
			if c := rec.last(); !strings.Contains(c.Text, "Duplicate of task "+itoa(old.ID)) || !strings.Contains(c.Text, "C1/50.1") {
				t.Fatalf("duplicate card: %q", c.Text)
			}
			if r.task.State != string(flow.Canceled) {
				t.Fatalf("the duplicate left the task in %s", r.task.State)
			}
		})
	}
}

// A `duplicate_of` that names no live task of the same tenant and repository goes back to Lead;
// nothing closes.
func TestLeadDuplicateOfATaskThatIsNoDuplicateGoesBack(t *testing.T) {
	for _, tc := range []struct{ name, state, tenant, repo string }{
		{"made up", "", "", ""},
		{"canceled", "canceled", "example-solo", "example/app"},
		{"rolled back", "rolled_back", "example-solo", "example/app"},
		{"another repository", "released", "example-solo", "example/other"},
		{"another tenant", "released", "example-team", "example/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := leadRig(t)
			ctx := t.Context()
			of := "999999"
			if tc.state != "" {
				old, err := r.q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: tc.tenant, Repo: tc.repo, Title: "Earlier"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = $2 WHERE id = $1`, old.ID, tc.state); err != nil {
					t.Fatal(err)
				}
				of = itoa(old.ID)
			}
			r.advance()
			r.leadSays(`{"type":"blocker_or_summary","text":"Same as before.","duplicate_of":` + of + `,"basis":{"checked":["the book search"],"confidence":"high"}}`)
			r.advance()
			if r.task.State == string(flow.Canceled) {
				t.Fatal("a wrong duplicate closed the task")
			}
		})
	}
}

// A task never duplicates itself, and unverified documents stay out of Lead's prompt.
func TestLeadPromptSkipsUnverifiedDocumentsAndItself(t *testing.T) {
	r, _ := leadRig(t)
	ctx := t.Context()
	ix := &search.Index{Pool: r.c.Pool}
	if _, err := ix.Add(ctx, search.Document{Tenant: "example-solo", Kind: "note", Source: "note:1", Title: "CSV export is forbidden", Body: "CSV export export", Status: "proposed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Add(ctx, search.Document{Tenant: "example-solo", Kind: "decision", Source: "docs/decisions.md", Title: "Exports use CSV", Body: "export CSV decision"}); err != nil {
		t.Fatal(err)
	}
	r.advance()
	p := string(r.leadJob().Payload)
	if strings.Contains(p, "CSV export is forbidden") || !strings.Contains(p, "[decision] Exports use CSV") || strings.Contains(p, "[task "+itoa(r.task.ID)+",") {
		t.Fatalf("prompt: %s", p)
	}
}

// releasedTask is an earlier, released task of the rig's tenant and repository, indexed for search.
func (r *rig) releasedTask(title string) bookdb.BookTask {
	r.t.Helper()
	ctx := r.t.Context()
	old, err := r.q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: title, ThreadRef: pgtype.Text{String: "C1/50.1", Valid: true}})
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := (&search.Index{Pool: r.c.Pool}).Add(ctx, search.Document{Tenant: "example-solo", Kind: "task", Source: "task:" + itoa(old.ID), Title: old.Title, Body: title}); err != nil {
		r.t.Fatal(err)
	}
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'released' WHERE id = $1`, old.ID); err != nil {
		r.t.Fatal(err)
	}
	return old
}

// Hidden scenarios live in the book for Lead and Verifier only: not in the spec card, not in
// Builder's prompt, not in a search made for Builder; a newer set supersedes the older one.
func TestHiddenScenariosStayAwayFromBuilder(t *testing.T) {
	r, rec := leadRig(t)
	ctx := t.Context()
	r.advance()
	const secret = "the export of 10001 rows keeps the header"
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},` +
		`"hidden_scenarios":[{"id":"HS-1","given":"a report of 10001 rows","when":"the owner exports it","then":"` + secret + `"}],"spec":` + leadSpec + `}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.last().Text, "10001") {
		t.Fatalf("the spec card shows a hidden scenario: %s", rec.last().Text)
	}
	ix := &search.Index{Pool: r.c.Pool}
	if hits, err := ix.Search(ctx, search.Query{Tenant: "example-solo", Text: "export rows", Visible: []string{"all"}}); err != nil || len(hits) != 0 {
		t.Fatalf("a search for Builder found %+v, %v", hits, err)
	}
	got, err := dispatch.HiddenScenarios(ctx, ix, "example-solo", r.task.ID)
	if err != nil || !strings.Contains(string(got), secret) {
		t.Fatalf("the Verifier's scenarios: %s, %v", got, err)
	}

	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(ctx, r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance() // Builder's job
	if p := string(r.lastJob().Payload); strings.Contains(p, "10001") || strings.Contains(p, "HS-1") {
		t.Fatalf("Builder's prompt carries a hidden scenario: %s", p)
	}
	if none, err := dispatch.HiddenScenarios(ctx, ix, "example-solo", 999999); err != nil || none != nil {
		t.Fatalf("a task without scenarios: %s, %v", none, err)
	}
}

// Hidden scenarios without a finished spec go back to Lead.
func TestHiddenScenariosWithoutASpecGoBack(t *testing.T) {
	r, _ := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"blocker_or_summary","text":"Maybe.","hidden_scenarios":[{"id":"HS-1","given":"a","when":"b","then":"c"}]}`)
	r.advance()
	evs, err := r.q.ListTaskEvents(t.Context(), pgtype.Int8{Int64: r.task.ID, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Kind == "lead.reply" {
			t.Fatalf("the owner got a reply: %s", e.Payload)
		}
	}
}

const goodPRD = "# CSV export\n\n## Problem\nOwners cannot take a report out of the product.\n\n## Goals\n- Download a report as CSV.\n\n## Non-goals\n- Excel files.\n\n## Acceptance Criteria\n- AC-1: given a report with rows, when the owner exports it, then a CSV with the same rows downloads.\n"

func specWithDocs(docs string) string {
	return `{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"documents":` + docs + `,"spec":` + leadSpec + `}`
}

// The quality loop: documents that pass go to the owner with the spec and then into Builder's
// prompt; a broken PRD goes back to Lead once, the owner sees no spec card, and the task returns
// to grilling; a second broken one tells the owner.
func TestDocumentsPassTheQualityLoopBeforeTheOwnerSeesTheSpec(t *testing.T) {
	r, rec := leadRig(t)
	ctx := t.Context()
	r.advance()
	bad, _ := json.Marshal([]map[string]string{{"path": "docs/prd/csv.md", "body": "# CSV\n\n## Problem\nx\n"}})
	r.leadSays(specWithDocs(string(bad)))
	r.advance()
	if r.task.State != "grilling" {
		t.Fatalf("a broken PRD left the task in %s", r.task.State)
	}
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if len(rec.cards) != 0 {
		t.Fatalf("the owner saw %d card(s) of a spec that failed the loop", len(rec.cards))
	}
	r.advance() // the correction job
	if p := string(r.leadJob().Payload); !strings.Contains(p, "missing section") || !strings.Contains(p, "docs/prd/csv.md") {
		t.Fatalf("Lead is not told what failed: %s", p)
	}

	good, _ := json.Marshal([]map[string]string{{"path": "docs/prd/csv.md", "body": goodPRD}})
	r.leadSays(specWithDocs(string(good)))
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("a good PRD left the task in %s", r.task.State)
	}
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(ctx, r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	if p := string(r.lastJob().Payload); !strings.Contains(p, "=== docs/prd/csv.md ===") || !strings.Contains(p, "Owners cannot take a report out") {
		t.Fatalf("Builder's prompt lacks the approved document: %s", p)
	}
}

// A revised spec without documents or hidden scenarios must not keep the ones of the first spec.
func TestRevisedSpecReplacesDocumentsAndHiddenScenarios(t *testing.T) {
	r, _ := leadRig(t)
	ctx := t.Context()
	r.advance()
	good, _ := json.Marshal([]map[string]string{{"path": "docs/prd/csv.md", "body": goodPRD}})
	first := specWithDocs(string(good))
	first = strings.Replace(first, `"documents":`, `"hidden_scenarios":[{"id":"HS-1","given":"a","when":"b","then":"c"}],"documents":`, 1)
	r.leadSays(first)
	r.advance()
	ix := &search.Index{Pool: r.c.Pool}
	if got, _ := dispatch.HiddenScenarios(ctx, ix, "example-solo", r.task.ID); got == nil {
		t.Fatal("no hidden scenarios after the first spec")
	}
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(ctx, r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "edit", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.say("owner_message", "drop the documents")
	r.advance()
	r.leadSays(specWithDocs(`[]`))
	r.advance()
	if got, err := dispatch.HiddenScenarios(ctx, ix, "example-solo", r.task.ID); err != nil || got != nil {
		t.Fatalf("old hidden scenarios survive: %s %v", got, err)
	}
	if err := inbox.Handle(ctx, r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	if p := string(r.lastJob().Payload); strings.Contains(p, "docs/prd/csv.md") {
		t.Fatalf("Builder is told to add a document of the first spec: %s", p)
	}

}

// An ADR that the doc-lint glob would skip (no number) is refused by the schema, so it never
// goes unlinted into Builder's prompt.
func TestUnnumberedAdrPathIsRefused(t *testing.T) {
	r2, _ := leadRig(t)
	r2.advance()
	loose, _ := json.Marshal([]map[string]string{{"path": "docs/adr/foo.md", "body": "x"}})
	r2.leadSays(specWithDocs(string(loose)))
	r2.advance()
	if r2.task.State != "grilling" {
		t.Fatalf("an unnumbered ADR path was accepted (state %s)", r2.task.State)
	}
}

const quotedSpec = `{"title":"CSV export","goal":"Export the report as CSV.","prd_path":"docs/prd/csv.md",
 "acceptance_criteria":[{"id":"AC-1","given":"a report","when":"the owner exports it","then":"a CSV file downloads","source_quote":"add csv EXPORT"},
 {"id":"AC-2","given":"an empty report","when":"the owner exports it","then":"nothing downloads"}],
 "tasks":[{"id":"T-1","title":"Add the export"}]}`

func leadSpecReply(spec string) string {
	return `{"type":"blocker_or_summary","understood":"A CSV export of the report.","text":"Spec is ready.","spec_ready":true,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + spec + `}`
}

// TestSpecQuoteMustBeTheOwnersWords: a criterion's source_quote that is in no owner message goes
// back to Lead; a real one (any case, any spacing) passes, and the card marks the criterion that has
// none as Lead's addition.
func TestSpecQuoteMustBeTheOwnersWords(t *testing.T) {
	r, rec := leadRig(t)
	r.advance()
	invented := strings.Replace(quotedSpec, "add csv EXPORT", "export everything nightly", 1)
	r.leadSays(leadSpecReply(invented))
	r.advance() // sends it back
	r.advance() // queues the correction
	if j := r.leadJob(); j.State != "queued" || !strings.Contains(string(j.Payload), "in no owner message") {
		t.Fatalf("the correction job is %s: %s", j.State, j.Payload)
	}
	if r.task.State != "grilling" {
		t.Fatalf("an invented quote left the task in %s", r.task.State)
	}
	r.leadSays(leadSpecReply(quotedSpec))
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("a real quote left the task in %s", r.task.State)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := rec.last(); !strings.Contains(c.Text, `[owner: "add csv EXPORT"]`) || !strings.Contains(c.Text, "AC-2: given an empty report, when the owner exports it, then nothing downloads [Lead's addition]") {
		t.Fatalf("spec card: %s", c.Text)
	}
}

// TestVerifierSeesTheSpecAndTheOwnersWords: the review job carries the approved spec and the
// message the owner typed, so a wrong reading shows against the original.
func TestVerifierSeesTheSpecAndTheOwnersWords(t *testing.T) {
	r, _ := leadRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.advance()
	r.leadSays(leadSpecReply(quotedSpec))
	r.advance()
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(t.Context(), r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.toReview("export.txt")
	r.advance()
	p := string(r.verifierJob().Payload)
	for _, want := range []string{"The spec the owner approved", "a CSV file downloads", "The owner's own words", "Add CSV export", "[Lead's addition]"} {
		if !strings.Contains(p, want) {
			t.Fatalf("the Verifier prompt lacks %q: %s", want, p)
		}
	}
}

// TestVerifierGetsEveryOwnerMessageAndNoSpecOfASplitTask: a quote from a later owner message
// reaches the Verifier with the message it came from, a spec made of several tasks is not given
// to one task's review, and a spec without quotes is not marked down as Lead's addition.
func TestVerifierGetsEveryOwnerMessageAndNoSpecOfASplitTask(t *testing.T) {
	r, _ := leadRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.advance()
	r.say("owner_message", "also export as JSON for the archive")
	later := strings.Replace(strings.Replace(quotedSpec, "add csv EXPORT", "also export as JSON", 1), "nothing downloads", "nothing downloads, JSON included", 1)
	r.leadSays(leadSpecReply(later))
	r.advance()
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(t.Context(), r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.toReview("export.txt")
	r.advance()
	p := string(r.verifierJob().Payload)
	if !strings.Contains(p, "also export as JSON for the archive") || !strings.Contains(p, "Add CSV export") {
		t.Fatalf("the Verifier prompt lacks an owner message: %s", p)
	}
}

func TestSpecTextMarksOnlySpecsThatQuote(t *testing.T) {
	plain := json.RawMessage(`{"title":"t","goal":"g","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c"}],"tasks":[{"id":"T-1","title":"x"}]}`)
	if got := dispatch.SpecText(plain, false); strings.Contains(got, "Lead's addition") {
		t.Fatalf("an old spec is marked: %s", got)
	}
	if got := dispatch.SpecText(plain, true); !strings.Contains(got, "Lead's addition") {
		t.Fatalf("the card does not mark it: %s", got)
	}
}

func TestVerifierGetsNoSpecOfASplitTask(t *testing.T) {
	r, _ := leadRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.advance()
	r.leadSays(leadSpecReply(quotedSpec))
	r.advance()
	inbox := &dispatch.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := inbox.Handle(t.Context(), r.tenant, integration.ChatEvent{Kind: integration.ChatButton, User: "U1", Action: "approve", TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.say("spec.expanded", "split")
	r.toReview("export.txt")
	r.advance()
	p := string(r.verifierJob().Payload)
	if strings.Contains(p, "The spec the owner approved") || !strings.Contains(p, "Add CSV export") {
		t.Fatalf("a split task's review prompt: %s", p)
	}
}

func TestCoverageMisses(t *testing.T) {
	spec := func(goal, non string) json.RawMessage {
		return json.RawMessage(`{"title":"CSV export","goal":"` + goal + `","non_goals":[` + non + `],"acceptance_criteria":[{"id":"AC-1","given":"a report","when":"it is exported","then":"a CSV downloads"}],"tasks":[{"id":"T-1","title":"x"}]}`)
	}
	owner := []string{"Export 42 rows as CSV, don't touch the login page, see report_v2 and the \"Overview\" tab."}
	for name, tc := range map[string]struct {
		spec json.RawMessage
		want []string
	}{
		"everything kept":      {spec("42 rows of report_v2 on the Overview tab", `"the login page"`), nil},
		"number dropped":       {spec("rows of report_v2 on the Overview tab", `"the login page"`), []string{"42"}},
		"identifier dropped":   {spec("42 rows on the Overview tab", `"the login page"`), []string{"report_v2"}},
		"quoted name dropped":  {spec("42 rows of report_v2", `"the login page"`), []string{"Overview"}},
		"limit dropped":        {spec("42 rows of report_v2 on the Overview tab", ""), []string{`a limit you stated ("don't")`}},
		"limit kept in a goal": {spec("42 rows of report_v2 on the Overview tab, never the login page", ""), nil},
	} {
		got := dispatch.CoverageMisses(owner, tc.spec)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
	if got := dispatch.CoverageMisses([]string{"Без страницы входа, 5 строк"}, spec("5 строк", "")); len(got) != 1 || !strings.Contains(got[0], "без") {
		t.Errorf("a Russian negation: %q", got)
	}
}

// TestCoverageMissGoesBackOnceThenShowsOnTheCard: a spec that drops a number is sent back to Lead;
// the same miss in the next spec reaches the owner with the list on the card instead of blocking.
func TestCoverageMissGoesBackOnceThenShowsOnTheCard(t *testing.T) {
	r, rec := leadRig(t)
	r.say("owner_message", "Make it 42 rows per page")
	r.advance()
	r.leadSays(leadSpecReply(quotedSpec))
	r.advance() // sends it back
	r.advance() // queues the correction
	if j := r.leadJob(); j.State != "queued" || !strings.Contains(string(j.Payload), "42") {
		t.Fatalf("the correction job is %s: %s", j.State, j.Payload)
	}
	r.leadSays(leadSpecReply(quotedSpec))
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("the second spec left the task in %s", r.task.State)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := rec.last(); !strings.Contains(c.Text, "Not found in the spec: 42") {
		t.Fatalf("spec card: %s", c.Text)
	}
}

// TestCoverageSendBackKeepsTheDocLoopsCorrection: a coverage miss is a soft rejection, so a broken
// PRD in the corrected spec still gets its own single correction instead of ending the job.
func TestCoverageSendBackKeepsTheDocLoopsCorrection(t *testing.T) {
	r, rec := leadRig(t)
	r.say("owner_message", "Make it 42 rows per page")
	r.advance()
	bad, _ := json.Marshal([]map[string]string{{"path": "docs/prd/csv.md", "body": "# CSV\n\n## Problem\nx\n"}})
	r.leadSays(specWithDocs(string(bad))) // no 42, broken PRD
	r.advance()
	r.advance() // the correction job
	if p := string(r.leadJob().Payload); !strings.Contains(p, "42") {
		t.Fatalf("Lead is not told about 42: %s", p)
	}
	fixed := strings.Replace(specWithDocs(string(bad)), "Export the report as CSV.", "Export 42 rows as CSV.", 1)
	r.leadSays(fixed)
	r.advance() // coverage passes, the loop finds the PRD
	r.advance() // the second correction job
	if j := r.leadJob(); j.State != "queued" || !strings.Contains(string(j.Payload), "missing section") {
		t.Fatalf("the doc loop's correction is %s: %s", j.State, j.Payload)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.cards {
		if strings.Contains(c.Text, "retry") {
			t.Fatalf("the owner heard of a failure: %s", c.Text)
		}
	}
}

func TestCoverageMissesAvoidsFalseAlarms(t *testing.T) {
	spec := func(extra string) json.RawMessage {
		return json.RawMessage(`{"title":"t","goal":"Retry 10 times and show 1000 rows.","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c","source_quote":"` + extra + `"}],"tasks":[{"id":"T-1","title":"x","repo":"github.com/acme/web"}]}`)
	}
	for name, tc := range map[string]struct {
		owner []string
		spec  json.RawMessage
		want  int
	}{
		"a bare option pick":       {[]string{"Make it 1000 rows", "2"}, spec(""), 0},
		"a thousands separator":    {[]string{"Show 1,000 rows"}, spec(""), 0},
		"a url":                    {[]string{"see https://example.com/docs?id=5 for 1000 rows"}, spec(""), 0},
		"a Russian bug report":     {[]string{"Кнопка экспорта не работает, почини"}, spec(""), 0},
		"a bug report in English":  {[]string{"The export button is not working"}, spec(""), 0},
		"a quote carries a name":   {[]string{"Переименуй кнопку «Сохранить»"}, spec("Переименуй кнопку «Сохранить»"), 0},
		"a repo path":              {[]string{"in acme/web only"}, spec(""), 0},
		"1 is not the 1 of 10":     {[]string{"retry 1 time"}, spec(""), 1},
		"a stated limit is a miss": {[]string{"do not touch the login page"}, spec(""), 1},
	} {
		if got := dispatch.CoverageMisses(tc.owner, tc.spec); len(got) != tc.want {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

func (r *rig) readerJobs() []bookdb.BookJob {
	r.t.Helper()
	jobs, err := r.q.ListTaskJobs(r.t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "reader"})
	if err != nil {
		r.t.Fatal(err)
	}
	return jobs
}

func (r *rig) readerSays(state, result string) {
	r.t.Helper()
	jobs := r.readerJobs()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET state = $2, exit_reason = CASE WHEN $2 = 'done' THEN 'done' ELSE 'error' END, result = $3, finished_at = now() WHERE id = $1`, jobs[len(jobs)-1].ID, state, []byte(result)); err != nil {
		r.t.Fatal(err)
	}
}

const leadSpecReady = `{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`

// TestIndependentReadingIsOffWithoutAReaderChain: the second reader is a switch, not a default.
func TestIndependentReadingIsOffWithoutAReaderChain(t *testing.T) {
	r, _ := leadRig(t)
	r.advance()
	r.leadSays(leadSpecReady)
	r.advance()
	if r.task.State != "spec" || len(r.readerJobs()) != 0 {
		t.Fatalf("state %s, %d reader jobs", r.task.State, len(r.readerJobs()))
	}
}

// TestIndependentReadingHoldsTheSpecAndPutsTheDifferenceOnTheCard: with a reader chain the spec waits
// for the reader (who sees only the owner's words), and a statement the spec does not carry is shown.
func TestIndependentReadingHoldsTheSpecAndPutsTheDifferenceOnTheCard(t *testing.T) {
	for name, tc := range map[string]struct {
		state, result string
		wantNote      bool
	}{
		"differs":       {"done", `{"done_means":["A CSV file downloads from the report page","Anonymous visitors see a pricing banner"]}`, true},
		"carried":       {"done", `{"done_means":["A CSV file downloads when the owner exports the report"]}`, false},
		"reader failed": {"failed", `{}`, false},
	} {
		t.Run(name, func(t *testing.T) { checkReading(t, tc.state, tc.result, tc.wantNote) })
	}
}

// TestReaderGetsNoRepositoryAndAsksForEnglish: the independent reader must not see the repository (its
// AGENTS.md would carry Lead's framing), and its statements are written in English for the diff.
func TestReaderGetsNoRepositoryAndAsksForEnglish(t *testing.T) {
	r, _ := leadRig(t)
	r.c.Roles["reader"] = config.Role{Name: "reader", OutputSchema: "schemas/reading.json"}
	r.c.Router.Chains["reader"] = []string{"fake"}
	r.advance()
	r.leadSays(leadSpecReady)
	r.advance()
	var job struct {
		Repo, Prompt string
		NoRepo       bool `json:"no_repo"`
		Instructions []string
	}
	if err := json.Unmarshal(r.readerJobs()[0].Payload, &job); err != nil {
		t.Fatal(err)
	}
	if job.Repo != "" || !job.NoRepo || len(job.Instructions) != 0 || !strings.Contains(job.Prompt, "in English") {
		t.Fatalf("reader job: %+v", job)
	}
}

func checkReading(t *testing.T, state, result string, wantNote bool) {
	t.Helper()
	r, rec := leadRig(t)
	r.c.Roles["reader"] = config.Role{Name: "reader", OutputSchema: "schemas/reading.json"}
	r.c.Router.Chains["reader"] = []string{"fake"}
	r.advance()
	r.leadSays(leadSpecReady)
	r.advance()
	if r.task.State != "grilling" || len(r.readerJobs()) != 1 {
		t.Fatalf("held spec: state %s, %d reader jobs", r.task.State, len(r.readerJobs()))
	}
	if p := string(r.readerJobs()[0].Payload); !strings.Contains(p, "Add CSV export") || strings.Contains(p, "Export the report as CSV") {
		t.Fatalf("the reader must see the owner's words and not the spec: %s", p)
	}
	r.advance() // still running: still held, no second job
	if r.task.State != "grilling" || len(r.readerJobs()) != 1 {
		t.Fatalf("while running: state %s, %d reader jobs", r.task.State, len(r.readerJobs()))
	}
	r.readerSays(state, result)
	r.advance()
	if r.task.State != "spec" {
		t.Fatalf("after the reading the task is in %s", r.task.State)
	}
	if _, err := r.c.Outbox.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	card := rec.last().Text
	if got := strings.Contains(card, "Readings differ"); got != wantNote {
		t.Fatalf("note on the card = %v, want %v:\n%s", got, wantNote, card)
	}
	if wantNote && !strings.Contains(card, "pricing banner") {
		t.Fatalf("the differing statement is missing:\n%s", card)
	}
}

// TestSpecCardOfAProblemReportShowsTheFindingAndItsButtons: the card says whether the problem was
// reproduced (a guess never reads as a finding) and offers Create task and Not needed.
func TestSpecCardOfAProblemReportShowsTheFindingAndItsButtons(t *testing.T) {
	spec := func(reproduced string) json.RawMessage {
		return json.RawMessage(`{"title":"fix: total","goal":"Total is 0.","prd_path":"docs/prd/x.md","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c"}],"tasks":[{"id":"T-1","title":"x"}],"report":{"priority":"P2","reproduced":` + reproduced + `,"steps":["build the report"],"evidence":["TestX fails"]}}`)
	}
	if got := dispatch.SpecText(spec("true"), true); !strings.Contains(got, "Problem reproduced, priority P2.") || !strings.Contains(got, "Evidence: TestX fails") {
		t.Errorf("reproduced card:\n%s", got)
	}
	if got := dispatch.SpecText(spec("false"), true); !strings.Contains(got, "Problem NOT reproduced") {
		t.Errorf("not reproduced card:\n%s", got)
	}
	labels := func(s json.RawMessage) string {
		var l []string
		for _, b := range dispatch.SpecButtons(s) {
			l = append(l, b.Action+":"+b.Label)
		}
		return strings.Join(l, ",")
	}
	if got := labels(spec("true")); got != "approve:Create task,edit:Edit,reject:Not needed" {
		t.Errorf("problem buttons: %s", got)
	}
	if got := labels(json.RawMessage(leadSpec)); got != "approve:Approve,edit:Edit,reject:Reject" {
		t.Errorf("idea buttons: %s", got)
	}
}
