package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/gitops"
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
	git   func(ctx context.Context, repo string) (gitops.Auth, error)
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
			git: func(ctx context.Context, repo string) (gitops.Auth, error) {
				token, err := app.Token(ctx, repo)
				return gitops.Auth{Token: token}, err
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
		git:   func(context.Context, string) (gitops.Auth, error) { return gitops.Auth{Token: token}, nil },
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
// so the token of a repository is never ambiguous.
func githubAccessFor(sec config.Secrets, tenants map[string]config.Tenant) (*access, error) {
	base, err := githubAccess(sec)
	if err != nil {
		return nil, err
	}
	own, names, err := tenantTokens(sec, tenants)
	if err != nil || len(own) == 0 {
		return base, err
	}
	pick := func(ctx context.Context, repo string) (string, error) {
		if token, ok := own[repoKey(repo)]; ok {
			return token, nil
		}
		if base == nil {
			return "", fmt.Errorf("github: no token for %s: its tenant has none and the instance has neither an App nor a token", repo)
		}
		auth, err := base.git(ctx, repo)
		return auth.Token, err
	}
	out := &access{
		kind:  "tenant tokens for " + strings.Join(names, ", "),
		forge: &github.Client{TokenFor: pick},
		git: func(ctx context.Context, repo string) (gitops.Auth, error) {
			token, err := pick(ctx, repo)
			return gitops.Auth{Token: token}, err
		},
	}
	if base != nil {
		out.kind = base.kind + "; " + out.kind
		out.forge.Token = base.forge.Token
	}
	return out, nil
}

// tenantTokens maps each repository of a tenant with its own token to that token.
func tenantTokens(sec config.Secrets, tenants map[string]config.Tenant) (map[string]string, []string, error) {
	own := map[string]string{}
	var names []string
	for name, t := range tenants {
		ts, err := sec.Tenant(name)
		if err != nil {
			return nil, nil, err
		}
		token, err := optionalSecret(ts, secretGitHubToken)
		if err != nil {
			return nil, nil, fmt.Errorf("tenant %s: %w", name, err)
		}
		if token == "" {
			continue
		}
		names = append(names, name)
		for _, repo := range t.Repos {
			own[repoKey(repo)] = token
		}
	}
	sort.Strings(names)
	return own, names, nil
}

// repoKey is a repository as owner/name in lower case, whether or not it names its host.
func repoKey(repo string) string {
	key := strings.ToLower(repo)
	if parts := strings.Split(key, "/"); len(parts) == 3 {
		key = parts[1] + "/" + parts[2]
	}
	return key
}
