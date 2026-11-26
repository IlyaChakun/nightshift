package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// TestATenantWithoutACredentialIsOffAlone: with no instance credential, a tenant without its
// own token is off and the conductor runs only the tenant that has one (review of PR #260;
// T1 regression).
func TestATenantWithoutACredentialIsOffAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSecret(t, dir, "tenants/example-solo/github_token", "solo-test-only", 0o400)
	acc, err := githubAccessFor(config.Secrets{Dir: dir}, exampleTenants)
	if err != nil || acc == nil {
		t.Fatalf("access %v, error %v", acc, err)
	}
	if !errors.Is(acc.off["example-team"], errNoCredential) {
		t.Fatalf("team is not off: %v", acc.off)
	}
	if on := acc.on(exampleTenants); len(on) != 1 || on["example-solo"].Name == "" {
		t.Fatalf("the conductor would run %v", slices.Collect(maps.Keys(on)))
	}

	none, err := githubAccessFor(config.Secrets{Dir: t.TempDir()}, exampleTenants)
	if err != nil || none != nil {
		t.Fatalf("no credential at all: access %v, error %v", none, err)
	}
}

// TestABadTenantTokenStopsOnlyThatTenant: a token file nightshift cannot use turns off its
// tenant, never the service, and never falls back to the instance's credential (review of
// PR #260; T1 regression).
func TestABadTenantTokenStopsOnlyThatTenant(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeSecret(t, dir, "github_token", "instance-test-only", 0o400)
	writeSecret(t, dir, "tenants/example-team/github_token", "team-test-only", 0o644)
	acc, err := githubAccessFor(config.Secrets{Dir: dir}, exampleTenants)
	if err != nil || acc == nil {
		t.Fatalf("access %v, error %v", acc, err)
	}
	if acc.off["example-team"] == nil || len(acc.off) != 1 {
		t.Fatalf("off %v", acc.off)
	}
	if _, err := acc.git(t.Context(), "example-org/example-team"); err == nil {
		t.Fatal("a tenant with a bad token file got the instance's token")
	}
	if auth, err := acc.git(t.Context(), "example-owner/example-solo"); err != nil || auth.Token != "instance-test-only" {
		t.Fatalf("solo's repository: %+v %v", auth, err)
	}
}

// TestATenantTokenBesideAnApp: an App instance with a tenant token uses the tenant's token for
// its repositories (T1).
func TestATenantTokenBesideAnApp(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	dir := t.TempDir()
	writeSecret(t, dir, "github_app_id", "12345", 0o400)
	writeSecret(t, dir, "github_app_key", string(pemKey), 0o400)
	writeSecret(t, dir, "tenants/example-solo/github_token", "solo-test-only", 0o400)
	acc, err := githubAccessFor(config.Secrets{Dir: dir}, exampleTenants)
	if err != nil || acc == nil {
		t.Fatalf("access %v, error %v", acc, err)
	}
	if len(acc.off) != 0 || !strings.HasPrefix(acc.kind, "GitHub App") {
		t.Fatalf("off %v, kind %q", acc.off, acc.kind)
	}
	if token, err := acc.forge.TokenFor(t.Context(), "example-owner/example-solo"); err != nil || token != "solo-test-only" {
		t.Fatalf("solo's repository: %q %v", token, err)
	}
}

var exampleTenants = map[string]config.Tenant{
	"example-solo": {Name: "example-solo", Repos: []string{"github.com/example-owner/example-solo"}},
	"example-team": {Name: "example-team", Repos: []string{"github.com/example-org/example-team"}},
}

func writeSecret(t *testing.T, dir, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}
