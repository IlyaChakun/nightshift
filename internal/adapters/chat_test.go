package adapters

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
)

func TestNewChat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{ChatBotToken, ChatAppToken} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"-test-only"), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	s, log := config.Secrets{Dir: dir}, slog.New(slog.DiscardHandler)
	if _, secrets, err := NewChat("slack-app", s, log); err != nil || len(secrets) != 2 {
		t.Errorf("slack-app: %v, %v", secrets, err)
	}
	if _, _, err := NewChat("fake", s, log); err != nil {
		t.Errorf("fake: %v", err)
	}
	if _, _, err := NewChat("slack-app", config.Secrets{Dir: t.TempDir()}, log); err == nil {
		t.Error("missing tokens must fail")
	}
	if _, _, err := NewChat("pigeon", s, log); err == nil {
		t.Error("unknown adapter must fail")
	}
}
