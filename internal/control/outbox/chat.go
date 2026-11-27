package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
)

// ChatTarget is the outbox target for chat cards.
const ChatTarget = "chat"

// ChatSender delivers chat cards from the outbox to each tenant's adapter.
type ChatSender map[string]chatapi.Chat

// Send implements Sender.
func (s ChatSender) Send(ctx context.Context, m Message) error {
	var card chatapi.Card
	if err := json.Unmarshal(m.Payload, &card); err != nil {
		return fmt.Errorf("%w: bad card: %w", ErrPermanent, err)
	}
	card.Delivery = m.ID
	chat, ok := s[m.Tenant]
	if !ok {
		return fmt.Errorf("%w: no chat for tenant %q", ErrPermanent, m.Tenant)
	}
	if _, err := chat.Post(ctx, card); err != nil {
		if permanentChatError(err) {
			return fmt.Errorf("%w: chat post: %w", ErrPermanent, err)
		}
		return fmt.Errorf("chat post: %w", err)
	}
	return nil
}

// permanentChatCodes are chat errors that a retry cannot fix: the card or the channel is wrong.
// not_in_channel is left out: inviting the bot fixes it while the message still retries.
var permanentChatCodes = []string{
	"invalid_blocks", "invalid_blocks_format", "msg_too_long", "channel_not_found", "is_archived",
}

func permanentChatError(err error) bool {
	msg := err.Error()
	return slices.ContainsFunc(permanentChatCodes, func(c string) bool { return strings.Contains(msg, c) })
}
