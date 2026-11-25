package main

import (
	"context"
	"errors"
	"io/fs"

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
