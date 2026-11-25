package deploy_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type service struct {
	Image       string            `yaml:"image"`
	Build       any               `yaml:"build"`
	ReadOnly    bool              `yaml:"read_only"`
	CapDrop     []string          `yaml:"cap_drop"`
	Secrets     []secretRef       `yaml:"secrets"`
	Networks    []string          `yaml:"networks"`
	Ports       []string          `yaml:"ports"`
	CapAdd      []string          `yaml:"cap_add"`
	Volumes     []string          `yaml:"volumes"`
	Environment map[string]string `yaml:"environment"`
	Command     []string          `yaml:"command"`
}

// secretRef is a service secret in the short form (a name) or the long one (source, target).
type secretRef struct{ Source, Target string }

func (r *secretRef) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode(&r.Source)
	}
	var long struct{ Source, Target string }
	if err := n.Decode(&long); err != nil {
		return err
	}
	r.Source, r.Target = long.Source, long.Target
	return nil
}

func holds(s service, secret string) bool {
	return slices.ContainsFunc(s.Secrets, func(r secretRef) bool { return r.Source == secret })
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
		if holds(s, secret) {
			t.Errorf("%s must not hold %s", name, secret)
		}
	}
}

// TestWorkerIsolation is the static part of the T6 escape tests: what the compose file lets
// the worker reach. The live checks (no GitHub, no host outside the allowlist, no local
// network) need containers and are part of the owner's phase 2 done check.
func TestWorkerIsolation(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c compose
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	w := c.Services["worker"]
	if !slices.Equal(w.Networks, []string{"jobs"}) || !c.Networks["jobs"].Internal {
		t.Errorf("the worker must sit only on the internal jobs network, got %v", w.Networks)
	}
	forbidSecrets(t, w, "worker", "pg_admin", "pg_owner", "pg_owner_password", "pg_nightshift", "pg_nightshift_password",
		"pg_backup_password", "chat_bot_token", "chat_app_token", "worker_token_sha256", "restic_password", "restic_repository")
	if !slices.Equal(w.CapDrop, []string{"ALL"}) || !slices.Equal(w.CapAdd, []string{"SETUID", "SETGID"}) {
		t.Errorf("the worker keeps only SETUID and SETGID, got drop %v add %v", w.CapDrop, w.CapAdd)
	}
	if !slices.Contains(w.Volumes, "repos:/repos:ro") {
		t.Error("the repository mirrors must be read-only in the worker")
	}
	// Only nightshift, the egress proxy and the Go module cache may be on both jobs and a network with a way out.
	for name, s := range c.Services {
		if slices.Contains(s.Networks, "jobs") && slices.Contains(s.Networks, "internet") && name != "nightshift" && name != "egress" && name != "modcache" {
			t.Errorf("%s bridges the jobs network and the internet", name)
		}
	}
	checkGoModules(t, w)
	checkToolchain(t, c)
	// The agent is another user in the same container: only a root-only directory of the image
	// keeps it away from the files on every host.
	for _, r := range w.Secrets {
		if !strings.HasPrefix(r.Target, "/run/worker-secrets/") {
			t.Errorf("worker secret %s must be mounted under /run/worker-secrets, got %q", r.Source, r.Target)
		}
	}
	dockerfile, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "mkdir -m 0700 /run/worker-secrets") {
		t.Error("the worker image must create /run/worker-secrets for root only")
	}
	if !holds(c.Services["nightshift"], "worker_token_sha256") || holds(c.Services["nightshift"], "worker_token") {
		t.Error("nightshift checks the worker token by its hash and never holds the token")
	}
}

// TestConductorMounts checks what the conductor needs from compose and the images: it writes
// the mirrors, reads the worker's artifacts and alone holds the GitHub token.
func TestConductorMounts(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c compose
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	dockerfile, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	w := c.Services["worker"]
	ns := c.Services["nightshift"]
	if !slices.Contains(ns.Volumes, "artifacts:/artifacts:ro") || !slices.Contains(ns.Volumes, "repos:/repos") {
		t.Errorf("the conductor reads artifacts and writes the mirrors, got %v", ns.Volumes)
	}
	for _, secret := range []string{"github_token", "github_app_id", "github_app_key"} {
		if holds(w, secret) || !holds(ns, secret) {
			t.Errorf("only nightshift holds %s", secret)
		}
	}
	for _, want := range []string{"chown 10001 /repos", "chown 0:10001 /artifacts", "chmod 2751 /artifacts"} {
		if strings.Count(string(dockerfile), want) != 2 {
			t.Errorf("both images must %q: a new volume takes its ownership from whichever mounts it first", want)
		}
	}
}

// checkGoModules: Go modules come from the cache service. The worker's allowlist stays closed to
// the host that large modules redirect to; only the cache service reaches it.
func checkGoModules(t *testing.T, w service) {
	t.Helper()
	if w.Environment["GOPROXY"] != "http://modcache:3000" {
		t.Errorf("the worker must fetch Go modules from modcache, got GOPROXY %q", w.Environment["GOPROXY"])
	}
	acl, err := os.ReadFile("egress-acl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(acl), "storage.googleapis.com") {
		t.Error("the worker's egress allowlist must not open storage.googleapis.com")
	}
}

// checkToolchain keeps the agent away from the tool hosts (ADR 0010): the install proxy sits on a
// network the worker is not on, and its hosts are not in the worker's allowlist.
func checkToolchain(t *testing.T, c compose) {
	t.Helper()
	if !c.Networks["install"].Internal {
		t.Error("the install network must be internal")
	}
	if slices.Contains(c.Services["worker"].Networks, "install") {
		t.Error("the worker must not be on the install network")
	}
	if n := c.Services["toolchain"].Networks; slices.Contains(n, "internet") || !slices.Contains(n, "install") || !slices.Contains(n, "jobs") {
		t.Errorf("toolchain must sit on jobs and install, never on the internet: %v", n)
	}
	if n := c.Services["egress-install"].Networks; slices.Contains(n, "jobs") || !slices.Contains(n, "install") || !slices.Contains(n, "internet") {
		t.Errorf("egress-install must bridge install and the internet, never jobs: %v", n)
	}
	if !strings.Contains(strings.Join(c.Services["worker"].Command, " "), "--toolchain-url") {
		t.Error("the worker must install toolchains through the toolchain service")
	}
	acl, err := os.ReadFile("egress-acl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"github.com", "go.dev", "nodejs.org", "mise.jdx.dev"} {
		if strings.Contains(string(acl), "- "+host+"\n") {
			t.Errorf("the worker's allowlist must not hold the tool host %s", host)
		}
	}
}
