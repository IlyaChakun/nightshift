package deploy_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/execution/runner"
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
	Profiles    []string          `yaml:"profiles"`
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

// TestGrafanaLoginOnlyInTheObsProfile: a plain `docker compose up` must not need the Grafana
// secret, so an install set up before it still starts.
func TestGrafanaLoginOnlyInTheObsProfile(t *testing.T) {
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
		for _, secret := range []string{"pg_grafana_password", "grafana_admin_password"} {
			if holds(s, secret) && !slices.Contains(s.Profiles, "obs") {
				t.Errorf("%s holds %s outside the obs profile", name, secret)
			}
		}
	}
}

// TestGrafanaNeedsALogin: lgtm sits on the jobs network beside the worker and reads the book,
// so Grafana must not let a visitor in without a login, and its admin password is a generated
// secret that start.sh also sets on a data volume created before it.
func TestGrafanaNeedsALogin(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c compose
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	g := c.Services["lgtm"]
	if g.Environment["GF_AUTH_ANONYMOUS_ENABLED"] != "false" {
		t.Errorf("lgtm must turn anonymous access off, got %q", g.Environment["GF_AUTH_ANONYMOUS_ENABLED"])
	}
	// $$ is compose's escape for $, so Grafana itself expands $__file.
	if want := "$$__file{/run/secrets/grafana_admin_password}"; g.Environment["GF_SECURITY_ADMIN_PASSWORD"] != want {
		t.Errorf("lgtm must take the admin password from its secret, got %q", g.Environment["GF_SECURITY_ADMIN_PASSWORD"])
	}
	if !holds(g, "grafana_admin_password") {
		t.Error("lgtm must hold grafana_admin_password")
	}
	if !slices.Equal(g.Command, []string{"sh", "/etc/grafana-nightshift/start.sh"}) ||
		!slices.Contains(g.Volumes, "./grafana/start.sh:/etc/grafana-nightshift/start.sh:ro") {
		t.Errorf("lgtm must start through grafana/start.sh, got command %v", g.Command)
	}
	script, err := os.ReadFile("grafana/start.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "reset-admin-password --password-from-stdin </run/secrets/grafana_admin_password") {
		t.Error("grafana/start.sh must set the admin password from the secret on an existing volume")
	}
	init, err := os.ReadFile("init-secrets.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(init), `put grafana_admin_password 0 "$(rand)"`) {
		t.Error("init-secrets.sh must generate grafana_admin_password")
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
	checkPreviewEgress(t, c)
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

// TestWorkerCannotReachTheBook is the static negative test of ADR 0026's "the worker has no book
// access": the Postgres server shares no network with the worker, and the worker gets no Postgres
// credential, database address, book volume or flag. Services that hold a book credential and sit
// on the jobs network (nightshift, lgtm) talk to Postgres themselves and forward nothing. The live
// check is docs/live-checks/A7-worker-book-isolation.md.
func TestWorkerCannotReachTheBook(t *testing.T) {
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
	pg, ok := c.Services["postgres"]
	if !ok || len(pg.Networks) == 0 || len(w.Networks) == 0 {
		t.Fatal("compose has no postgres or worker service with networks; the check is broken")
	}
	for _, n := range pg.Networks {
		if slices.Contains(w.Networks, n) {
			t.Errorf("postgres shares network %s with the worker", n)
		}
	}
	for _, r := range w.Secrets {
		if strings.HasPrefix(r.Source, "pg_") {
			t.Errorf("the worker holds the Postgres credential %s", r.Source)
		}
	}
	forbidBookSettings(t, w)
	for _, v := range w.Volumes {
		if name, _, _ := strings.Cut(v, ":"); slices.Contains(bookVolumes(c, w), name) {
			t.Errorf("the worker mounts %s, which holds book data or dumps", v)
		}
	}
}

// bookVolumes are the named volumes of services that hold a Postgres or backup credential and
// share no network with the worker: the data directory, the dumps and their copies.
func bookVolumes(c compose, w service) []string {
	var out []string
	for _, s := range c.Services {
		holder := slices.ContainsFunc(s.Secrets, func(r secretRef) bool {
			return strings.HasPrefix(r.Source, "pg_") || strings.HasPrefix(r.Source, "restic_")
		})
		if !holder || slices.ContainsFunc(s.Networks, func(n string) bool { return slices.Contains(w.Networks, n) }) {
			continue
		}
		for _, v := range s.Volumes {
			if name, _, _ := strings.Cut(v, ":"); !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "/") {
				out = append(out, name)
			}
		}
	}
	return out
}

// forbidBookSettings fails on a database address or flag the worker gets.
func forbidBookSettings(t *testing.T, w service) {
	t.Helper()
	for k, v := range w.Environment {
		if strings.HasPrefix(k, "PG") || strings.Contains(k, "DATABASE") || strings.Contains(v, "postgres") {
			t.Errorf("the worker gets a database setting %s=%s", k, v)
		}
	}
	for _, arg := range w.Command {
		if strings.Contains(arg, "postgres") || strings.Contains(arg, "database") {
			t.Errorf("the worker command names the book: %s", arg)
		}
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
	install, err := os.ReadFile("egress-install-acl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(install), "- proxy.golang.org") {
		t.Error("the toolchain service fetches Go tools from modcache, not from the public proxy")
	}
	if strings.Contains(string(acl), "storage.googleapis.com") {
		t.Error("the worker's egress allowlist must not open storage.googleapis.com")
	}
}

// checkPreviewEgress keeps the preview path narrow (ADR 0028): egress-preview has no way out,
// only it and the deployer's previews share the preview network, the worker's plain http goes
// to it, and its allowlist holds the preview suffix alone.
func checkPreviewEgress(t *testing.T, c compose) {
	t.Helper()
	if !c.Networks["preview"].Internal {
		t.Error("the preview network must be internal")
	}
	if n := c.Services["egress-preview"].Networks; !slices.Equal(n, []string{"jobs", "preview"}) {
		t.Errorf("egress-preview must sit on jobs and preview only: %v", n)
	}
	for name, s := range c.Services {
		if name != "egress-preview" && slices.Contains(s.Networks, "preview") {
			t.Errorf("%s is on the preview network", name)
		}
	}
	if cmd := strings.Join(c.Services["egress-preview"].Command, " "); !strings.Contains(cmd, "--allow-range ${NIGHTSHIFT_PREVIEW_SUBNET:-172.31.254.0/24}") || strings.Count(cmd, "--allow-") != 1 {
		t.Errorf("egress-preview may allow the preview subnet and nothing else: %s", cmd)
	}
	env := c.Services["worker"].Environment
	if env["HTTP_PROXY"] != "http://egress-preview:4750" || env["http_proxy"] != env["HTTP_PROXY"] || env["no_proxy"] != env["NO_PROXY"] {
		t.Errorf("the worker's plain http must go to egress-preview, in both spellings: %v", env)
	}
	raw, err := os.ReadFile("egress-preview-acl.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var acl struct {
		Default struct {
			Action  string   `yaml:"action"`
			Allowed []string `yaml:"allowed_domains"`
		} `yaml:"default"`
		Services []any `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &acl); err != nil {
		t.Fatal(err)
	}
	if acl.Default.Action != "enforce" || !slices.Equal(acl.Default.Allowed, []string{"*.preview.internal"}) || len(acl.Services) != 0 {
		t.Errorf("egress-preview-acl.yaml must enforce *.preview.internal only: %+v", acl)
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
	if env := c.Services["toolchain"].Environment; env["GOPROXY"] != "http://modcache:3000" || !strings.Contains(env["NO_PROXY"], "modcache") {
		t.Errorf("toolchain must fetch Go tools from the module cache: %v", env)
	}
	if !slices.Contains(c.Services["modcache"].Networks, "jobs") {
		t.Error("modcache must share the jobs network with toolchain")
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

// TestWorkerStopGraceCoversDelivery: on `docker compose stop` the worker ends its job and then
// sends the result and the reason of the failure within runner.DeliverTimeout. A stop grace
// shorter than that lets Docker kill the worker first, and the book keeps a running job with no
// reason. Before delivery the worker stops the agent (up to 6 s, harness), a running git (up to
// 10 s, gitops) and removes the checkout (unbounded; the margin is an estimate the live check in
// docs/live-checks/G5-shutdown-reason.md measures).
func TestWorkerStopGraceCoversDelivery(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Services map[string]struct {
			StopGrace string `yaml:"stop_grace_period"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	grace, err := time.ParseDuration(c.Services["worker"].StopGrace)
	if err != nil {
		t.Fatalf("the worker needs a stop_grace_period: %v", err)
	}
	if want := runner.DeliverTimeout + 25*time.Second; grace < want {
		t.Errorf("worker stop_grace_period is %s, want at least %s", grace, want)
	}
}
