package dispatch

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
		if got := WantsRussian(tc.chat, tc.msg); got != tc.ru {
			t.Errorf("WantsRussian(%q, %q) = %v, want %v", tc.chat, tc.msg, got, tc.ru)
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
