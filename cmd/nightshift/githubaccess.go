package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/trustedgit"
)

const (
	secretGitHubAppID  = "github_app_id"  //nolint:gosec // G101: a secret's file name
	secretGitHubAppKey = "github_app_key" //nolint:gosec // G101: a secret's file name
)

// access is how nightshift reaches the code host: the API client and the git credential of
// each repository.
type access struct {
	kind  string // how it signs in, for the doctor
	forge *github.Client
	git   func(ctx context.Context, repo string) (trustedgit.Auth, error)
	off   map[string]error // tenants without a usable credential, and why
}

// githubAccess prefers the GitHub App, whose pull requests a person can review, and falls back
// to a token. It returns nil without either: that is the documented way to keep the conductor
// off. Half an App, or a key that cannot be read, is an error.
func githubAccess(sec config.Secrets) (*access, error) {
	id, idErr := optionalSecret(sec, secretGitHubAppID)
	key, keyErr := optionalSecret(sec, secretGitHubAppKey)
	if err := errors.Join(idErr, keyErr); err != nil {
		return nil, err
	}
	if id != "" || key != "" {
		app, err := github.NewApp(id, key)
		if err != nil {
			return nil, err
		}
		return &access{
			kind:  "GitHub App",
			forge: &github.Client{TokenFor: app.Token},
			git: func(ctx context.Context, repo string) (trustedgit.Auth, error) {
				token, err := app.Token(ctx, repo)
				return trustedgit.Auth{Token: token}, err
			},
		}, nil
	}
	token, err := optionalSecret(sec, secretGitHubToken)
	if err != nil || token == "" {
		return nil, err
	}
	return &access{
		kind:  "token (you cannot approve its pull requests; the App is better)",
		forge: &github.Client{Token: token},
		git:   func(context.Context, string) (trustedgit.Auth, error) { return trustedgit.Auth{Token: token}, nil },
	}, nil
}

// optionalSecret reads a secret that may be missing or empty.
func optionalSecret(sec config.Secrets, name string) (string, error) {
	value, err := sec.Read(name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, config.ErrEmptySecret) {
		return "", nil
	}
	return value, err
}

// githubAccessFor is githubAccess with the tenants' own tokens: a tenant whose secrets hold
// tenants/<name>/github_token reaches its repositories with it, the others with the
// instance's App or token (design section 17, "Secrets"). A repository belongs to one tenant,
// so the token of a repository is never ambiguous. A tenant with no credential, or with a token
// file that cannot be read, is listed in off with the reason; the other tenants are not held
// back by it. It returns nil when there is no credential anywhere; with only unreadable tenant
// files it returns an access whose tenants are all off.
func githubAccessFor(sec config.Secrets, tenants map[string]config.Tenant) (*access, error) {
	base, err := githubAccess(sec)
	if err != nil {
		return nil, err
	}
	own, names, off := tenantTokens(sec, tenants, base != nil)
	if len(own) == 0 && len(off) == 0 {
		return base, nil
	}
	if len(names) == 0 && base == nil && allMissing(off) {
		return base, nil // nil: no tenant has a credential
	}
	offRepo := map[string]error{}
	for name, reason := range off {
		for _, repo := range tenants[name].Repos {
			offRepo[repoKey(repo)] = reason
		}
	}
	pick := func(ctx context.Context, repo string) (string, error) {
		key := repoKey(repo)
		if token, ok := own[key]; ok {
			return token, nil
		}
		if reason, ok := offRepo[key]; ok {
			return "", fmt.Errorf("github: no token for %s: %w", repo, reason)
		}
		if base == nil {
			return "", fmt.Errorf("github: no token for %s: no tenant names it and the instance has neither an App nor a token", repo)
		}
		auth, err := base.git(ctx, repo)
		return auth.Token, err
	}
	out := &access{
		kind:  "tenant tokens for " + strings.Join(names, ", "),
		forge: &github.Client{TokenFor: pick},
		git: func(ctx context.Context, repo string) (trustedgit.Auth, error) {
			token, err := pick(ctx, repo)
			return trustedgit.Auth{Token: token}, err
		},
		off: off,
	}
	if len(names) == 0 {
		out.kind = "no tenant tokens"
	}
	if base != nil {
		out.kind = base.kind + "; " + out.kind
		out.forge.Token = base.forge.Token
	}
	return out, nil
}

// errNoCredential marks a tenant that has neither its own token nor an instance credential.
var errNoCredential = errors.New("neither its own github_token nor an instance App or token")

func allMissing(off map[string]error) bool {
	for _, reason := range off {
		if !errors.Is(reason, errNoCredential) {
			return false
		}
	}
	return true
}

// tenantTokens maps each repository of a tenant with its own token to that token. A tenant
// whose token cannot be read, or that has none while the instance has no credential either,
// goes to off: a bad file never falls back to the instance's credential.
func tenantTokens(sec config.Secrets, tenants map[string]config.Tenant, instance bool) (own map[string]string, names []string, off map[string]error) {
	own, off = map[string]string{}, map[string]error{}
	for _, name := range slices.Sorted(maps.Keys(tenants)) {
		token, err := tenantToken(sec, name)
		switch {
		case err != nil:
			off[name] = err
			continue
		case token == "":
			if !instance {
				off[name] = errNoCredential
			}
			continue
		}
		names = append(names, name)
		for _, repo := range tenants[name].Repos {
			own[repoKey(repo)] = token
		}
	}
	return own, names, off
}

func tenantToken(sec config.Secrets, name string) (string, error) {
	ts, err := sec.Tenant(name)
	if err != nil {
		return "", err
	}
	token, err := optionalSecret(ts, secretGitHubToken)
	if err != nil {
		return "", fmt.Errorf("tenants/%s/%s: %w", name, secretGitHubToken, err)
	}
	return token, nil
}

// on are the tenants that have a credential.
func (a *access) on(tenants map[string]config.Tenant) map[string]config.Tenant {
	out := make(map[string]config.Tenant, len(tenants))
	for name, t := range tenants {
		if _, off := a.off[name]; !off {
			out[name] = t
		}
	}
	return out
}

// repoKey is a repository as owner/name in lower case, whether or not it names its host.
func repoKey(repo string) string {
	key := strings.ToLower(repo)
	if parts := strings.Split(key, "/"); len(parts) == 3 {
		key = parts[1] + "/" + parts[2]
	}
	return key
}
