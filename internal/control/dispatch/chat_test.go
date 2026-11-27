package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

func TestTitle(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"  Add CSV export\nwith a header": "Add CSV export",
		"":                                "(empty message)",
		strings.Repeat("я", 200):          strings.Repeat("я", maxTitle-1) + "…",
	}
	for in, want := range tests {
		if got := title(in); got != want {
			t.Errorf("title(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRouteTenantFor(t *testing.T) {
	t.Parallel()
	a := config.Tenant{Name: "example-solo", Chat: config.Adapter{Channels: map[string]string{"planning": "C1"}}}
	b := config.Tenant{Name: "example-team", Chat: config.Adapter{Channels: map[string]string{"planning": "C2"}}}
	shared := Route{Tenants: []config.Tenant{a, b}}
	if got, ok := shared.TenantFor("C2"); !ok || got.Name != "example-team" {
		t.Errorf("C2 → %q, %v", got.Name, ok)
	}
	if _, ok := shared.TenantFor("C9"); ok {
		t.Error("an unknown channel on a shared adapter must not match")
	}
	if got, ok := (Route{Tenants: []config.Tenant{a}}).TenantFor("C9"); !ok || got.Name != "example-solo" {
		t.Errorf("single tenant → %q, %v", got.Name, ok)
	}
}

func TestReplyText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		created bool
		msg     string
		want    string
	}{
		{"new, ru", true, "Тест 2: добавь в README раздел про запуск", "Принял, это задача #7. Lead подготовит спеку и пришлёт её сюда на согласование."},
		{"new, en", true, "Add CSV export", "Got it, this is task #7. Lead will draft a spec and post it here for your OK."},
		{"follow-up, ru", false, "Уточнение: раздел короткий, 5 строк", "Учёл в задаче #7."},
		{"follow-up, en", false, "also a footer", "Added to task #7."},
		{"ru with english terms", true, "Сделай retry для webhook", "Принял, это задача #7. Lead подготовит спеку и пришлёт её сюда на согласование."},
		{"empty defaults to en", false, "", "Added to task #7."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := replyText(tt.created, 7, isRussian(tt.msg))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, tt.msg) && tt.msg != "" {
				t.Errorf("reply echoes the message: %q", got)
			}
		})
	}
}

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

// TestChatLanguageFollowsTheTenantSetting: `language.chat: auto` follows the message, a code fixes
// the fixed texts (Russian or English) and tells Lead which language to write in.
func TestChatLanguageFollowsTheTenantSetting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		chat, msg string
		ru        bool
	}{
		{"auto", "Сделай retry", true}, {"", "Сделай retry", true}, {"auto", "Add retry", false},
		{"ru", "Add retry", true}, {"en", "Сделай retry", false}, {"de", "Сделай retry", false},
	} {
		if got := wantsRussian(tc.chat, tc.msg); got != tc.ru {
			t.Errorf("wantsRussian(%q, %q) = %v, want %v", tc.chat, tc.msg, got, tc.ru)
		}
	}
	if got := languageNote("auto", nil); got != "" {
		t.Errorf("auto with no message must add no note: %q", got)
	}
	for chat, want := range map[string]string{"ru": "Russian", "en": "English", "de": "`de`"} {
		if got := languageNote(chat, nil); !strings.Contains(got, want) {
			t.Errorf("languageNote(%q) = %q, want it to name %s", chat, got, want)
		}
	}
}
