// Package fake is the chat twin for tests: it records posted cards and replays events
// that a test injects, with the same contract as the real adapters.
package fake

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
)

// Chat implements chatapi.Chat in memory.
type Chat struct {
	mu     sync.Mutex
	posted []chatapi.Card
	events chan chatapi.ChatEvent
	gone   chan struct{}
	once   sync.Once
	next   int
	reacts []Reaction
}

var (
	_ chatapi.Chat    = (*Chat)(nil)
	_ chatapi.Reactor = (*Chat)(nil)
)

// Reaction is an emoji the twin was asked to put on a message.
type Reaction struct {
	Msg   chatapi.MessageRef
	Emoji string
}

// New returns a twin with a buffered event stream.
func New() *Chat {
	return &Chat{events: make(chan chatapi.ChatEvent, 64), gone: make(chan struct{})}
}

// Post records the card; a card without a thread starts a new one.
func (c *Chat) Post(_ context.Context, card chatapi.Card) (chatapi.MessageRef, error) {
	if card.Thread.Channel == "" {
		return chatapi.MessageRef{}, errors.New("fake chat: card has no channel")
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

// Events returns the stream of injected events; it closes when ctx ends or after Disconnect.
func (c *Chat) Events(ctx context.Context) (<-chan chatapi.ChatEvent, error) {
	out := make(chan chatapi.ChatEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.gone:
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

// Disconnect ends every event stream, as a real adapter does when its connection gives up.
func (c *Chat) Disconnect() { c.once.Do(func() { close(c.gone) }) }

// Inject delivers an event as if a person did it in chat.
func (c *Chat) Inject(ev chatapi.ChatEvent) { c.events <- ev }

// Posted returns a copy of every card posted so far.
func (c *Chat) Posted() []chatapi.Card {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]chatapi.Card(nil), c.posted...)
}

// React records the reaction.
func (c *Chat) React(_ context.Context, msg chatapi.MessageRef, emoji string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reacts = append(c.reacts, Reaction{Msg: msg, Emoji: emoji})
	return nil
}

// Reactions returns a copy of every reaction so far.
func (c *Chat) Reactions() []Reaction {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Reaction(nil), c.reacts...)
}
