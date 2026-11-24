package dispatch_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func soloTenant() config.Tenant {
	t := config.Tenant{Name: "example-solo", Governance: "solo", Repos: []string{"github.com/example-owner/example-solo"}}
	t.People.Owner.Chat = "U0OWNER"
	t.People.Approvers = []string{"owner"}
	return t
}

// startService runs a service on the chat twin and returns a stop function that waits
// for the graceful shutdown.
func startService(t *testing.T, pool *pgxpool.Pool, chat *fake.Chat) func() {
	t.Helper()
	tenant := soloTenant()
	log := slog.New(slog.DiscardHandler)
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: dispatch.ChatSender{tenant.Name: chat}},
		Mask: dispatch.NewMasker(), Log: log,
	}
	s := &dispatch.Service{
		Pool: pool, Routes: []dispatch.Route{{Chat: chat, Tenants: []config.Tenant{tenant}}},
		Inbox: &dispatch.Inbox{Pool: pool, Outbox: o, Log: log}, Outbox: o,
		FlushEvery: time.Second, StopTimeout: 10 * time.Second, Log: log,
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	}
}

func waitPosted(t *testing.T, chat *fake.Chat, n int) []integration.Card {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got := chat.Posted(); len(got) >= n {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("want %d posted cards, got %+v", n, chat.Posted())
	return nil
}

func message(user, thread, id, text string) integration.ChatEvent {
	return integration.ChatEvent{
		Kind: integration.ChatMessage, User: user, Text: text, MessageID: id,
		Thread: integration.MessageRef{Channel: "C1", ID: thread}, At: time.Now(),
	}
}

// TestServiceAnswersOwnerInThread is the T4-lite check of phase 1: an owner message on
// the chat twin gets a reply in its thread, and the task survives a restart.
func TestServiceAnswersOwnerInThread(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	chat := fake.New()

	stop := startService(t, pool, chat)
	chat.Inject(message("U0STRANGER", "99.1", "99.1", "delete everything"))
	chat.Inject(message("U0OWNER", "100.1", "100.1", "Add CSV export\nwith a header row"))
	got := waitPosted(t, chat, 1)
	if got[0].Thread != (integration.MessageRef{Channel: "C1", ID: "100.1"}) || got[0].Text != "Got it, this is task #1. Lead will draft a spec and post it here for your OK." {
		t.Fatalf("reply: %+v", got[0])
	}
	if r := chat.Reactions(); len(r) != 1 || r[0].Emoji != "eyes" || r[0].Msg != (integration.MessageRef{Channel: "C1", ID: "100.1"}) {
		t.Fatalf("reactions: %+v (want one eyes on the owner message, none on the stranger's)", r)
	}
	stop()

	// A new process on the same book finds the task by its thread.
	stop = startService(t, pool, chat)
	defer stop()
	chat.Inject(message("U0OWNER", "100.1", "100.2", "also a footer"))
	chat.Inject(message("U0OWNER", "100.1", "100.2", "also a footer")) // a redelivery
	got = waitPosted(t, chat, 2)
	if got[1].Text != "Added to task #1." || got[1].Thread.ID != "100.1" {
		t.Fatalf("reply after restart: %+v", got[1])
	}

	tasks, err := bookdb.New(pool).ListTasksByState(t.Context(), bookdb.ListTasksByStateParams{Tenant: "example-solo", State: "idea"})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks: %+v, %v", tasks, err)
	}
	events, err := bookdb.New(pool).ListTaskEvents(t.Context(), pgtype.Int8{Int64: tasks[0].ID, Valid: true})
	if err != nil || len(events) < 2 {
		t.Fatalf("events: %+v, %v", events, err)
	}
	time.Sleep(300 * time.Millisecond) // let a wrong extra reply show up
	for _, c := range chat.Posted() {
		if strings.Contains(c.Text, "delete") || c.Thread.ID == "99.1" {
			t.Fatalf("answered a stranger: %+v", c)
		}
	}
	if n := len(chat.Posted()); n != 2 {
		t.Fatalf("want 2 replies, got %d: %+v", n, chat.Posted())
	}
}

// TestServiceStopsWhenChatCloses checks that a chat connection that gives up stops the
// service with ErrChatClosed, so compose restarts it instead of leaving the bot deaf.
func TestServiceStopsWhenChatCloses(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	chat := fake.New()
	tenant := soloTenant()
	log := slog.New(slog.DiscardHandler)
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: dispatch.ChatSender{tenant.Name: chat}},
		Mask: dispatch.NewMasker(), Log: log,
	}
	s := &dispatch.Service{
		Pool: pool, Routes: []dispatch.Route{{Chat: chat, Tenants: []config.Tenant{tenant}}},
		Inbox: &dispatch.Inbox{Pool: pool, Outbox: o, Log: log}, Outbox: o,
		FlushEvery: time.Second, StopTimeout: 10 * time.Second, Log: log,
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(t.Context()) }()
	chat.Disconnect()
	select {
	case err := <-done:
		if !errors.Is(err, dispatch.ErrChatClosed) {
			t.Fatalf("run: got %v, want ErrChatClosed", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("service still running after its chat closed")
	}
}
