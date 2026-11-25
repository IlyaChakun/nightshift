package slackapp

import (
	"errors"
	"net/url"
	"strings"
)

// newAppURL is the page that creates an app; with a manifest it arrives filled in.
const newAppURL = "https://api.slack.com/apps?new_app=1&manifest_yaml="

// CreateURL is the link that opens the app creation dialog filled in from the manifest
// (deploy/slack-manifest.yaml), so nothing is pasted by hand. Comment lines are left out to
// keep the link short. A manifest holds no secret.
func CreateURL(manifest string) (string, error) {
	var kept []string
	for line := range strings.Lines(manifest) {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	body := strings.Join(kept, "")
	if !strings.Contains(body, "socket_mode_enabled: true") {
		return "", errors.New("slackapp: the manifest must enable socket mode: the factory opens no inbound port")
	}
	// Spaces as %20: the page reads a plus sign as itself.
	return newAppURL + strings.ReplaceAll(url.QueryEscape(body), "+", "%20"), nil
}
