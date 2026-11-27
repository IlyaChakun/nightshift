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

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// Chat implements chatapi.Chat on the book.
type Chat struct{ Pool *pgxpool.Pool }

var _ chatapi.Chat = (*Chat)(nil)

// New returns the adapter.
func New(pool *pgxpool.Pool) *Chat { return &Chat{Pool: pool} }

// Post stores the card in its task's thread. Posting the same card again, as the outbox does
// after a crash, stores nothing new.
func (c *Chat) Post(ctx context.Context, card chatapi.Card) (chatapi.MessageRef, error) {
	id, err := strconv.ParseInt(card.TaskID, 10, 64)
	if err != nil {
		return chatapi.MessageRef{}, fmt.Errorf("inbox chat: card has no task id: %w", err)
	}
	payload, err := json.Marshal(card)
	if err != nil {
		return chatapi.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	q := bookdb.New(c.Pool)
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: id, Valid: true})
	if err != nil {
		return chatapi.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	// A redelivery carries the id of its outbox message; a new card with the same text does not.
	// Without an id (a direct caller) only the newest stored card is compared.
	for _, e := range slices.Backward(events) {
		if e.Kind != chatapi.InboxEventCard {
			continue
		}
		var last chatapi.Card
		if json.Unmarshal(e.Payload, &last) != nil {
			continue
		}
		if card.Delivery != 0 && last.Delivery == card.Delivery {
			return card.Thread, nil
		}
		if card.Delivery == 0 {
			// The book stores JSON in its own normal form: compare the decoded card.
			if reflect.DeepEqual(last, card) {
				return card.Thread, nil
			}
			break
		}
	}
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: card.Tenant, TaskID: pgtype.Int8{Int64: id, Valid: true}, Kind: chatapi.InboxEventCard, Actor: "nightshift", Payload: payload,
	}); err != nil {
		return chatapi.MessageRef{}, fmt.Errorf("inbox chat: %w", err)
	}
	return card.Thread, nil
}

// Events never delivers anything: the doors call the factory directly, with the same checks
// as a Slack event. The stream ends with ctx.
func (c *Chat) Events(ctx context.Context) (<-chan chatapi.ChatEvent, error) {
	if c.Pool == nil {
		return nil, errors.New("inbox chat: no book")
	}
	out := make(chan chatapi.ChatEvent)
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}
