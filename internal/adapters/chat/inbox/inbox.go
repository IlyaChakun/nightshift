// Package inbox is the chat adapter that lives in the book (ADR 0006): a posted card is an
// event of its task's thread, and the doors (terminal, MCP) read and answer it. A tenant
// without Slack sets `chat: { adapter: inbox }`.
package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// EventCard is the event kind of a stored card.
const EventCard = "inbox.card"

// Channel is the thread channel of every inbox thread.
const Channel = "inbox"

// Chat implements integration.Chat on the book.
type Chat struct{ Pool *pgxpool.Pool }

var _ integration.Chat = (*Chat)(nil)

// New returns the adapter.
func New(pool *pgxpool.Pool) *Chat { return &Chat{Pool: pool} }

// Post stores the card in its task's thread. Posting the same card again, as the outbox does
// after a crash, stores nothing new.
func (c *Chat) Post(ctx context.Context, card integration.Card) (integration.MessageRef, error) {
	id, err := strconv.ParseInt(card.TaskID, 10, 64)
	if err != nil {
		return integration.MessageRef{}, fmt.Errorf("inbox chat: card has no task id: %w", err)
	}
	payload, err := json.Marshal(card)
	if err != nil {
		return integration.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	q := bookdb.New(c.Pool)
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: id, Valid: true})
	if err != nil {
		return integration.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	for _, e := range slices.Backward(events) {
		if e.Kind == EventCard {
			// The book stores JSON in its own normal form: compare the decoded card.
			var last integration.Card
			if json.Unmarshal(e.Payload, &last) == nil && reflect.DeepEqual(last, card) {
				return card.Thread, nil
			}
			break
		}
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: card.Tenant, TaskID: pgtype.Int8{Int64: id, Valid: true}, Kind: EventCard, Actor: "nightshift", Payload: payload,
	}); err != nil {
		return integration.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	return card.Thread, nil
}

// Events never delivers anything: the doors call the factory directly, with the same checks
// as a Slack event. The stream ends with ctx.
func (c *Chat) Events(ctx context.Context) (<-chan integration.ChatEvent, error) {
	if c.Pool == nil {
		return nil, errors.New("inbox chat: no book")
	}
	out := make(chan integration.ChatEvent)
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}
