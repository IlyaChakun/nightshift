package doors

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

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
			got := replyText(tt.created, 7, chatapi.IsRussian(tt.msg))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, tt.msg) && tt.msg != "" {
				t.Errorf("reply echoes the message: %q", got)
			}
		})
	}
}
