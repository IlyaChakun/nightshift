package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
)

func TestGitHubAccess(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		files   map[string]string
		on      bool
		wantErr bool
	}{
		"nothing":            {},
		"empty files":        {files: map[string]string{"github_token": "", "github_app_id": "", "github_app_key": ""}},
		"token":              {files: map[string]string{"github_token": "test-only"}, on: true},
		"half an app":        {files: map[string]string{"github_app_id": "12345", "github_token": "test-only"}, wantErr: true},
		"app with a bad key": {files: map[string]string{"github_app_id": "12345", "github_app_key": "not pem"}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for file, value := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(value), 0o400); err != nil {
					t.Fatal(err)
				}
			}
			got, err := githubAccess(config.Secrets{Dir: dir})
			if (err != nil) != tc.wantErr || (got != nil) != tc.on {
				t.Errorf("access %v, error %v", got, err)
			}
		})
	}
}

// TestATenantTokenReachesOnlyItsRepositories: a tenant with its own token uses it for its
// repositories, whatever the host or case; the other tenant falls back to the instance's
// token (design section 17, "Secrets"; T1).
func TestATenantTokenReachesOnlyItsRepositories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(value), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	tenants := map[string]config.Tenant{
		"example-solo": {Name: "example-solo", Repos: []string{"github.com/example-owner/example-solo"}},
		"example-team": {Name: "example-team", Repos: []string{"github.com/example-org/example-team"}},
	}
	write("tenants/example-solo/github_token", "solo-test-only")
	only, err := githubAccessFor(config.Secrets{Dir: dir}, tenants)
	if err != nil || only == nil {
		t.Fatalf("access %v, error %v", only, err)
	}
	if auth, err := only.git(t.Context(), "Example-Owner/example-solo"); err != nil || auth.Token != "solo-test-only" {
		t.Fatalf("solo's repository: %+v %v", auth, err)
	}
	if _, err := only.git(t.Context(), "example-org/example-team"); err == nil {
		t.Fatal("a repository without a token got one")
	}

	write("github_token", "instance-test-only")
	both, err := githubAccessFor(config.Secrets{Dir: dir}, tenants)
	if err != nil {
		t.Fatal(err)
	}
	if auth, err := both.git(t.Context(), "example-org/example-team"); err != nil || auth.Token != "instance-test-only" {
		t.Fatalf("team's repository: %+v %v", auth, err)
	}
	if token, err := both.forge.TokenFor(t.Context(), "example-owner/example-solo"); err != nil || token != "solo-test-only" {
		t.Fatalf("the API client used %q %v for solo's repository", token, err)
	}
}
