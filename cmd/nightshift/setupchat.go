package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/slackapp"
)

// setupChatApp prints the link that creates the chat app from its manifest, and what stays
// manual: the two tokens.
func setupChatApp(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup chat-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifest := fs.String("manifest", "deploy/slack-manifest.yaml", "the app manifest")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	raw, err := os.ReadFile(*manifest)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift setup chat-app:", err)
		return exitFailure
	}
	link, err := slackapp.CreateURL(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift setup chat-app:", err)
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, `1. Open this link, pick the workspace and create the app:
   %s
2. Install App > Install to Workspace; the bot token (xoxb-) goes into deploy/secrets/chat_bot_token.
3. Basic Information > App-Level Tokens > add connections:write; the token (xapp-) goes into
   deploy/secrets/chat_app_token.
4. Invite the bot to your channel and check with `+"`nightshift doctor`"+`.
`, link)
	return exitOK
}
