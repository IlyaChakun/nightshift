package outbox

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
)

type failingChat struct{ err error }

func (f failingChat) Post(context.Context, chatapi.Card) (chatapi.MessageRef, error) {
	return chatapi.MessageRef{}, f.err
}
func (failingChat) React(context.Context, chatapi.MessageRef, string) error { return nil }
func (failingChat) Events(context.Context) (<-chan chatapi.ChatEvent, error) {
	return nil, errors.New("no events")
}

func TestChatSenderMarksUnfixableSlackErrorsPermanent(t *testing.T) {
	for _, tc := range []struct {
		err       string
		permanent bool
	}{
		{"invalid_blocks", true}, {"channel_not_found", true}, {"is_archived", true},
		{"ratelimited", false}, {"internal_error", false},
	} {
		s := ChatSender{"example-solo": failingChat{errors.New("slackapp: post: " + tc.err)}}
		err := s.Send(t.Context(), Message{Tenant: "example-solo", Payload: []byte(`{"task_id":"1"}`)})
		if got := errors.Is(err, ErrPermanent); got != tc.permanent {
			t.Errorf("%s: permanent = %v, want %v", tc.err, got, tc.permanent)
		}
	}
}
