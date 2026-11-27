// Package adapters picks the integration a tenant names in tenant.yaml.
package adapters

import (
	"fmt"
	"log/slog"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/fake"
	"github.com/ilyachakun/nightshift/internal/adapters/chat/slackapp"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// Chat secret names under /run/secrets; they are file names, not credentials.
const (
	ChatBotToken = "chat_bot_token" //nolint:gosec // G101: a secret's file name
	ChatAppToken = "chat_app_token" //nolint:gosec // G101: a secret's file name
)

// NewChat builds a chat adapter by name. It also returns the secret values it read, so
// the caller can mask them everywhere.
func NewChat(name string, s config.Secrets, log *slog.Logger) (chatapi.Chat, []string, error) {
	switch name {
	case "slack-app":
		bot, err := s.Read(ChatBotToken)
		if err != nil {
			return nil, nil, err
		}
		app, err := s.Read(ChatAppToken)
		if err != nil {
			return nil, nil, err
		}
		return slackapp.New(slackapp.Options{BotToken: bot, AppToken: app, Log: log}), []string{bot, app}, nil
	case "fake":
		return fake.New(), nil, nil
	}
	return nil, nil, fmt.Errorf("adapters: unknown chat adapter %q", name)
}
