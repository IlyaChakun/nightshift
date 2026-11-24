package dispatch

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
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
