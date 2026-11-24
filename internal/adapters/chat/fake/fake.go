// Package fake is the chat twin for tests: it records posted cards and replays events
// that a test injects, with the same contract as the real adapters.
package fake

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Chat implements integration.Chat in memory.
type Chat struct {
	mu     sync.Mutex
	posted []integration.Card
	events chan integration.ChatEvent
	next   int
}

var _ integration.Chat = (*Chat)(nil)

// New returns a twin with a buffered event stream.
func New() *Chat {
	return &Chat{events: make(chan integration.ChatEvent, 64)}
}

// Post records the card; a card without a thread starts a new one.
func (c *Chat) Post(_ context.Context, card integration.Card) (integration.MessageRef, error) {
	if card.Thread.Channel == "" {
		return integration.MessageRef{}, errors.New("fake chat: card has no channel")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if card.Thread.ID == "" {
		c.next++
		card.Thread.ID = "T" + strconv.Itoa(c.next)
	}
	c.posted = append(c.posted, card)
	return card.Thread, nil
}

// Events returns the stream of injected events; it closes when ctx ends.
func (c *Chat) Events(ctx context.Context) (<-chan integration.ChatEvent, error) {
	out := make(chan integration.ChatEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-c.events:
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// Inject delivers an event as if a person did it in chat.
func (c *Chat) Inject(ev integration.ChatEvent) { c.events <- ev }

// Posted returns a copy of every card posted so far.
func (c *Chat) Posted() []integration.Card {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]integration.Card(nil), c.posted...)
}
