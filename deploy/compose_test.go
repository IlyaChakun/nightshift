package deploy_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type service struct {
	Image    string   `yaml:"image"`
	Build    any      `yaml:"build"`
	ReadOnly bool     `yaml:"read_only"`
	CapDrop  []string `yaml:"cap_drop"`
	Secrets  []string `yaml:"secrets"`
	Networks []string `yaml:"networks"`
	Ports    []string `yaml:"ports"`
}

type compose struct {
	Services map[string]service `yaml:"services"`
	Networks map[string]struct {
		Internal bool `yaml:"internal"`
	} `yaml:"networks"`
}

// TestCompose checks the rules of design section 3 that a typo in compose would break.
func TestCompose(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c compose
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	for name, s := range c.Services {
		if s.Build == nil && !strings.Contains(s.Image, "@sha256:") {
			t.Errorf("%s: image %q is not pinned by digest", name, s.Image)
		}
		for _, p := range s.Ports {
			if !strings.HasPrefix(p, "127.0.0.1:") {
				t.Errorf("%s: port %q is published beyond localhost", name, p)
			}
		}
		if s.Build != nil && (!s.ReadOnly || !slices.Equal(s.CapDrop, []string{"ALL"})) {
			t.Errorf("%s: our images run read-only without capabilities", name)
		}
	}
	// Least privilege: migrations run in the migrate step, dumps as the read-only book_backup role.
	forbidSecrets(t, c.Services["nightshift"], "nightshift", "pg_admin", "pg_owner", "pg_owner_password")
	forbidSecrets(t, c.Services["backup"], "backup", "pg_admin")
	if !c.Networks["core"].Internal || !slices.Equal(c.Services["postgres"].Networks, []string{"core"}) {
		t.Error("Postgres must sit only on the internal core network")
	}
}

func forbidSecrets(t *testing.T, s service, name string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if slices.Contains(s.Secrets, secret) {
			t.Errorf("%s must not hold %s", name, secret)
		}
	}
}
