package fake

import (
	"context"
	"testing"

	"github.com/ilyachakun/nightshift/internal/integration"
)

func TestTwin(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := New()
	ref, err := c.Post(ctx, integration.Card{Text: "hi", Thread: integration.MessageRef{Channel: "planning"}})
	if err != nil || ref.ID != "T1" {
		t.Fatalf("post: %+v, %v", ref, err)
	}
	if _, err := c.Post(ctx, integration.Card{Text: "reply", Thread: ref}); err != nil {
		t.Fatal(err)
	}
	if got := c.Posted(); len(got) != 2 || got[1].Thread != ref {
		t.Fatalf("posted: %+v", got)
	}
	events, _ := c.Events(ctx)
	c.Inject(integration.ChatEvent{Kind: integration.ChatMessage, User: "U0OWNER", Text: "go"})
	if ev := <-events; ev.Text != "go" {
		t.Fatalf("event: %+v", ev)
	}
	cancel()
	if _, open := <-events; open {
		t.Error("stream must close when the context ends")
	}
}
