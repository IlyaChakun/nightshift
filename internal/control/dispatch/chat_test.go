package dispatch

import (
	"strings"
	"testing"
)

// TestLanguageNoteFollowsTheTenantSetting: a chat language code tells Lead which language to write
// in; `auto` with no message adds nothing (chatapi.WantsRussian picks the fixed texts).
func TestLanguageNoteFollowsTheTenantSetting(t *testing.T) {
	t.Parallel()
	if got := languageNote("auto", nil); got != "" {
		t.Errorf("auto with no message must add no note: %q", got)
	}
	for chat, want := range map[string]string{"ru": "Russian", "en": "English", "de": "`de`"} {
		if got := languageNote(chat, nil); !strings.Contains(got, want) {
			t.Errorf("languageNote(%q) = %q, want it to name %s", chat, got, want)
		}
	}
}
