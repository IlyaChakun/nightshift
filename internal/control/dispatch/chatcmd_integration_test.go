package dispatch_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/fake"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/knowledge/memory"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

func TestSlashCommandsAnswerInTheChannel(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	tenant := soloTenant()
	chat := fake.New()
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: dispatch.ChatSender{tenant.Name: chat}},
		Mask: dispatch.NewMasker(), Log: slog.New(slog.DiscardHandler),
	}
	in := &dispatch.Inbox{Pool: pool, Outbox: o}
	say := func(user, text string) {
		t.Helper()
		ev := integration.ChatEvent{Kind: integration.ChatCommand, User: user, Text: text, Thread: integration.MessageRef{Channel: "C1"}}
		if err := in.Handle(ctx, tenant, ev); err != nil {
			t.Fatal(err)
		}
	}
	last := func() string {
		t.Helper()
		posted := chat.Posted()
		if len(posted) == 0 {
			t.Fatal("no answer")
		}
		if c := posted[len(posted)-1]; c.Thread.Channel != "C1" {
			t.Fatalf("the answer went to %q", c.Thread.Channel)
		} else {
			return c.Text
		}
		return ""
	}

	say("U0OWNER", "memory")
	if !strings.Contains(last(), "memory is empty") {
		t.Fatalf("empty page: %s", last())
	}
	say("U0OWNER", "remember tests run with make test on the build machine")
	if !strings.Contains(last(), "Remembered") {
		t.Fatalf("remember: %s", last())
	}
	say("U0OWNER", "memory")
	if got := last(); !strings.Contains(got, "tests run with make test") || !strings.Contains(got, "chat:U0OWNER") {
		t.Fatalf("memory: %s", got)
	}
	page, err := memory.Store{Pool: pool}.Read(ctx, tenant.Name)
	if err != nil || len(page.Entries) != 1 || !strings.HasPrefix(page.Entries[0].Source, "owner:") {
		t.Fatalf("page %+v %v", page, err)
	}
	say("U0OWNER", "status")
	if !strings.Contains(last(), "Nothing waits for you") {
		t.Fatalf("status: %s", last())
	}
	say("U0OWNER", "dance")
	if !strings.Contains(last(), "Commands:") {
		t.Fatalf("usage: %s", last())
	}
	say("U0OWNER", "remember")
	if !strings.Contains(last(), "Say what to remember") {
		t.Fatalf("empty fact: %s", last())
	}

	// Anyone else gets no answer and changes nothing.
	n := len(chat.Posted())
	say("U0STRANGER", "remember the staging password is hunter2")
	if len(chat.Posted()) != n {
		t.Fatal("a stranger got an answer")
	}
	if page, _ = (memory.Store{Pool: pool}).Read(ctx, tenant.Name); len(page.Entries) != 1 {
		t.Fatalf("a stranger changed the memory page: %+v", page.Entries)
	}
}

// `/ns search` answers with the best hits of the tenant's book, and asks for words when it gets none.
func TestSearchCommandAnswersWithHits(t *testing.T) {
	ctx := t.Context()
	pool := pgtest.Start(t).ServicePool(t)
	tenant := config.Tenant{Name: "example-solo"}
	tenant.People.Owner.Chat = "U0OWNER"
	chat := fake.New()
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: dispatch.ChatSender{tenant.Name: chat}},
		Mask: dispatch.NewMasker(), Log: slog.New(slog.DiscardHandler),
	}
	in := &dispatch.Inbox{Pool: pool, Outbox: o}
	say := func(text string) string {
		t.Helper()
		if err := in.Handle(ctx, tenant, integration.ChatEvent{Kind: integration.ChatCommand, User: "U0OWNER", Text: text, Thread: integration.MessageRef{Channel: "C1"}}); err != nil {
			t.Fatal(err)
		}
		posted := chat.Posted()
		return posted[len(posted)-1].Text
	}
	if got := say("search"); !strings.Contains(got, "Say what to look for") {
		t.Fatalf("empty search: %s", got)
	}
	if _, err := (&search.Index{Pool: pool}).Add(ctx, search.Document{Tenant: tenant.Name, Kind: "decision", Source: "docs/decisions.md", Title: "Previews run on compose", Body: "Decision 13: previews run on the local compose."}); err != nil {
		t.Fatal(err)
	}
	if got := say("search compose previews"); !strings.Contains(got, "Previews run on compose") || !strings.Contains(got, "[decision]") {
		t.Fatalf("search: %s", got)
	}
}
