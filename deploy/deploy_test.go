// Package deploy_test checks the deploy files that the factory relies on.
package deploy_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type acl struct {
	Version string `yaml:"version"`
	Default struct {
		Action         string   `yaml:"action"`
		AllowedDomains []string `yaml:"allowed_domains"`
	} `yaml:"default"`
	Services []any `yaml:"services"`
}

func readACL(t *testing.T, path string) acl {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixed paths in the repository
	if err != nil {
		t.Fatal(err)
	}
	var a acl
	if err := yaml.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestEgressACLs(t *testing.T) {
	t.Parallel()
	// Both settings were found on the first real start: without them Smokescreen exits at
	// once, or refuses every request.
	settings, err := os.ReadFile("egress-config.yaml")
	if err != nil || !strings.Contains(string(settings), "\nallow_missing_role: true\n") {
		t.Errorf("egress-config.yaml must allow requests without a role: %v", err)
	}
	for _, path := range []string{"egress-acl.yaml", "egress-ci-acl.yaml", "egress-install-acl.yaml"} {
		a := readACL(t, path)
		if a.Services == nil {
			t.Errorf("%s: Smokescreen needs a services list, even an empty one", path)
		}
		if a.Version != "v1" || a.Default.Action != "enforce" || len(a.Services) != 0 {
			t.Errorf("%s: want version v1, default action enforce and no services", path)
		}
		for _, d := range a.Default.AllowedDomains {
			if d == "*" || strings.HasPrefix(d, "*.") && strings.Count(d, ".") < 3 {
				t.Errorf("%s: wildcard %q is too broad", path, d)
			}
		}
	}
	// Only the module cache reaches the host that large Go modules redirect to.
	if slices.Contains(readACL(t, "egress-install-acl.yaml").Default.AllowedDomains, "storage.googleapis.com") {
		t.Error("egress-install-acl.yaml opens storage.googleapis.com")
	}
	// The CI runner executes PR code: it must never reach model APIs.
	ci := readACL(t, "egress-ci-acl.yaml")
	if slices.Contains(ci.Default.AllowedDomains, "api.anthropic.com") {
		t.Error("egress-ci-acl.yaml allows a model API")
	}
}

func TestSlackManifest(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("slack-manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Settings struct {
			SocketMode bool `yaml:"socket_mode_enabled"`
		} `yaml:"settings"`
		Features struct {
			AppHome struct {
				Home bool `yaml:"home_tab_enabled"`
			} `yaml:"app_home"`
		} `yaml:"features"`
	}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if !m.Settings.SocketMode || !m.Features.AppHome.Home {
		t.Error("the app must use Socket Mode (no inbound ports) and the Home tab (control panel)")
	}
}
