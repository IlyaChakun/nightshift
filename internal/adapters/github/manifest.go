package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// NewAppURL is where a browser posts an app manifest for the signed-in account.
const NewAppURL = "https://github.com/settings/apps/new"

// Manifest describes the factory's GitHub App, so a person registers it with one click
// instead of filling the form: the permissions of design section 16, no webhook, private,
// and never Workflows or Administration.
func Manifest(name, homepage, redirect string) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"name": name, "url": homepage, "public": false, "redirect_url": redirect,
		// A hook address is required even though the hook is off: nothing calls in.
		"hook_attributes": map[string]any{"url": homepage, "active": false},
		"default_permissions": map[string]string{
			"contents": "write", "pull_requests": "write", "statuses": "write", "checks": "read", "actions": "read", "security_events": "read",
		},
		"default_events": []string{},
	})
	if err != nil {
		return "", fmt.Errorf("github app manifest: %w", err)
	}
	return string(raw), nil
}

// Registered is the app GitHub created from a manifest. PrivateKey is a secret.
type Registered struct {
	ID         string
	Slug       string
	Page       string // the app's page, where it is installed
	PrivateKey string
}

var codeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ConvertManifest trades the one-time code GitHub redirected back with for the new app's id
// and private key. The code is the only credential the call needs.
func (c *Client) ConvertManifest(ctx context.Context, code string) (Registered, error) {
	if !codeRe.MatchString(code) {
		return Registered{}, errors.New("github app manifest: bad code")
	}
	var out struct {
		ID      int64  `json:"id"`
		Slug    string `json:"slug"`
		HTMLURL string `json:"html_url"`
		PEM     string `json:"pem"`
	}
	err := c.do(ctx, http.MethodPost, "/app-manifests/"+url.PathEscape(code)+"/conversions", nil, &out)
	if err != nil {
		// The answer can hold the new key, so only the status is reported.
		if refused, ok := errors.AsType[*Error](err); ok {
			return Registered{}, fmt.Errorf("github app manifest: status %d: the code is used up or older than an hour", refused.Status)
		}
		return Registered{}, errors.New("github app manifest: the conversion failed")
	}
	if out.ID == 0 || out.PEM == "" {
		return Registered{}, errors.New("github app manifest: the answer has no id or key")
	}
	return Registered{ID: strconv.FormatInt(out.ID, 10), Slug: out.Slug, Page: out.HTMLURL, PrivateKey: out.PEM}, nil
}
