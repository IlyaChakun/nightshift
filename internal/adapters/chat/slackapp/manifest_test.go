package slackapp_test

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/slackapp"
)

// The link carries the manifest of the repository, readable back exactly.
func TestCreateURL(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../../deploy/slack-manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	link, err := slackapp.CreateURL(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(link, "+") || !strings.HasPrefix(link, "https://api.slack.com/apps?new_app=1&manifest_yaml=") {
		t.Errorf("link: %s", link)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	got := u.Query().Get("manifest_yaml")
	if !strings.HasPrefix(got, "display_information:") || !strings.Contains(got, "chat:write") || strings.Contains(got, "# ") {
		t.Errorf("manifest in the link:\n%s", got)
	}
	if _, err := slackapp.CreateURL("display_information: {}\n"); err == nil {
		t.Error("a manifest without socket mode must be refused")
	}
}
