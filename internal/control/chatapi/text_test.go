package chatapi

import (
	"strings"
	"testing"
)

func TestTitle(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"  Add CSV export\nwith a header": "Add CSV export",
		"":                                "(empty message)",
		strings.Repeat("я", 200):          strings.Repeat("я", MaxTitle-1) + "…",
	}
	for in, want := range tests {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWantsRussianFollowsTheTenantSetting: `language.chat: auto` follows the message, a code fixes
// the fixed texts (Russian or English).
func TestWantsRussianFollowsTheTenantSetting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		chat, msg string
		ru        bool
	}{
		{"auto", "Сделай retry", true}, {"", "Сделай retry", true}, {"auto", "Add retry", false},
		{"ru", "Add retry", true}, {"en", "Сделай retry", false}, {"de", "Сделай retry", false},
	} {
		if got := WantsRussian(tc.chat, tc.msg); got != tc.ru {
			t.Errorf("WantsRussian(%q, %q) = %v, want %v", tc.chat, tc.msg, got, tc.ru)
		}
	}
}
