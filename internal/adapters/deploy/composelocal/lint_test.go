package composelocal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const checkout = "/work/example-solo"

func recipe(service string) []byte {
	return []byte(`{"name":"ns","services":{"web":` + service + `}}`)
}

// T6: each forbidden setting is refused, and a plain recipe passes.
func TestLintRefusesEachForbiddenSetting(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, svc, rule string }{
		{"privileged", `{"image":"x","privileged":true}`, "privileged mode"},
		{"host network", `{"image":"x","network_mode":"host"}`, "host namespace"},
		{"host pid", `{"image":"x","pid":"host"}`, "host namespace"},
		{"host ipc", `{"image":"x","ipc":"host"}`, "host namespace"},
		{"another container's network", `{"image":"x","network_mode":"container:db"}`, "host namespace"},
		{"added capability", `{"image":"x","cap_add":["SYS_ADMIN"]}`, "added capabilities"},
		{"device", `{"image":"x","devices":[{"source":"/dev/kvm","target":"/dev/kvm"}]}`, "devices"},
		{"seccomp off", `{"image":"x","security_opt":["seccomp=unconfined"]}`, "security option"},
		{"docker socket", `{"image":"x","volumes":[{"type":"bind","source":"/var/run/docker.sock","target":"/s"}]}`, "docker socket"},
		{"docker socket inside the checkout", `{"image":"x","volumes":[{"type":"bind","source":"/work/example-solo/docker.sock","target":"/s"}]}`, "docker socket"},
		{"host home", `{"image":"x","volumes":[{"type":"bind","source":"/home/owner/.ssh","target":"/k"}]}`, "bind mount outside the checkout"},
		{"climbing out of the checkout", `{"image":"x","volumes":[{"type":"bind","source":"/work/example-solo/../secret","target":"/k"}]}`, "bind mount outside the checkout"},
		{"a sibling with the same prefix", `{"image":"x","volumes":[{"type":"bind","source":"/work/example-solo-other","target":"/k"}]}`, "bind mount outside the checkout"},
		{"relative source", `{"image":"x","volumes":[{"type":"bind","source":"../x","target":"/k"}]}`, "bind mount outside the checkout"},
		{"device cgroup rule", `{"image":"x","device_cgroup_rules":["b 8:* rwm"]}`, "device cgroup rules"},
		{"volumes from a foreign container", `{"image":"x","volumes_from":["container:db"]}`, "volumes from a container outside the project"},
		{"seccomp replaced", `{"image":"x","security_opt":["seccomp=/tmp/p.json"]}`, "security option"},
		{"selinux label off", `{"image":"x","security_opt":["label:disable"]}`, "security option"},
		{"host gateway", `{"image":"x","extra_hosts":{"h":"host-gateway"}}`, "host gateway"},
		{"runtime", `{"image":"x","runtime":"nvidia"}`, "engine setting"},
		{"cgroup parent", `{"image":"x","cgroup_parent":"/"}`, "engine setting"},
		{"build context is the host root", `{"build":{"context":"/"}}`, "build context outside the checkout"},
		{"build ssh agent", `{"build":{"context":"/work/example-solo","ssh":["default"]}}`, "build option"},
		{"build entitlements", `{"build":{"context":"/work/example-solo","entitlements":["security.insecure"]}}`, "build option"},
		{"extra build context outside", `{"build":{"context":"/work/example-solo","additional_contexts":{"x":"/etc"}}}`, "build context outside the checkout"},
		{"env file outside", `{"image":"x","env_file":[{"path":"/home/owner/.env"}]}`, "env file outside the checkout"},
		{"a provider service runs a program of the project", `{"provider":{"type":"sh","options":{}}}`, "recipe key not allowed"},
		{"the engine socket by a switch", `{"image":"x","use_api_socket":true}`, "recipe key not allowed"},
		{"lifecycle hooks", `{"image":"x","post_start":[{"command":"id","user":"root","privileged":true}]}`, "recipe key not allowed"},
		{"a label file", `{"image":"x","label_file":["/work/example-solo/l"]}`, "recipe key not allowed"},
		{"a fixed container name", `{"image":"x","container_name":"db"}`, "recipe key not allowed"},
		{"deploy block with device reservations", `{"image":"x","deploy":{"resources":{"reservations":{"devices":[{"driver":"nvidia"}]}}}}`, "recipe key not allowed"},
		{"a key that does not exist yet", `{"image":"x","tomorrow":true}`, "recipe key not allowed"},
		{"build privileged", `{"build":{"context":"/work/example-solo","privileged":true}}`, "recipe key not allowed"},
		{"another project's network by network_mode", `{"image":"x","network_mode":"other_default"}`, "network mode"},
		{"the default bridge by network_mode", `{"image":"x","network_mode":"bridge"}`, "network mode"},
		{"a service that is not in the recipe", `{"image":"x","network_mode":"service:db"}`, "network mode"},
		{"no process ceiling", `{"image":"x","pids_limit":-1}`, "limit removed"},
		{"zero processes means no ceiling", `{"image":"x","pids_limit":0}`, "limit removed"},
		{"unlimited swap", `{"image":"x","memswap_limit":"-1"}`, "limit removed"},
		{"a file build context", `{"build":{"context":"file:///etc"}}`, "build context outside the checkout"},
		{"a dockerfile outside", `{"build":{"context":"/work/example-solo","dockerfile":"/etc/passwd"}}`, "build file outside the checkout"},
		{"a dockerfile climbing out", `{"build":{"context":"/work/example-solo","dockerfile":"../x/Dockerfile"}}`, "build file outside the checkout"},
		{"unknown mount type", `{"image":"x","volumes":[{"type":"npipe","source":"x","target":"/k"}]}`, "mount type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Lint(recipe(tt.svc), checkout, "ns")
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range got {
				if v.Rule == tt.rule {
					return
				}
			}
			t.Errorf("violations = %v, want rule %q", got, tt.rule)
		})
	}
}

func TestLintPassesAPlainRecipe(t *testing.T) {
	t.Parallel()
	svc := `{"image":"x","ports":[{"target":80,"published":"8080"}],"volumes":[
	  {"type":"bind","source":"/work/example-solo/web/static","target":"/s"},
	  {"type":"volume","source":"data","target":"/d"},{"type":"tmpfs","target":"/t"}],
	  "cap_add":null,"security_opt":["no-new-privileges:true"]}`
	got, err := Lint(recipe(svc), checkout, "ns")
	if err != nil || len(got) != 0 {
		t.Fatalf("violations = %v, %v; want none", got, err)
	}
}

func TestLintNeverPassesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]string{
		"yaml":        "services:\n  web:\n    privileged: true\n",
		"no services": `{"name":"ns"}`,
		"empty":       ``,
	} {
		if v, err := Lint([]byte(doc), checkout, "ns"); err == nil {
			t.Errorf("%s: Lint accepted it with violations %v", name, v)
		}
	}
}

func TestLintNamesTheServiceAndReportsEveryProblem(t *testing.T) {
	t.Parallel()
	got, _ := Lint([]byte(`{"services":{"a":{"privileged":true},"b":{"network_mode":"host","cap_add":["NET_ADMIN"]}}}`), checkout, "ns")
	var s []string
	for _, v := range got {
		s = append(s, v.String())
	}
	all := strings.Join(s, "\n")
	for _, want := range []string{`service "a"`, `service "b"`, "NET_ADMIN", "network_mode: host"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
}

func TestLintTopLevelResources(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, top, rule string }{
		{"volume backed by the host root", `"volumes":{"h":{"driver":"local","driver_opts":{"type":"none","o":"bind","device":"/"}}}`, "volume backed by a host path"},
		{"external volume", `"volumes":{"h":{"external":true,"name":"prod-data"}}`, "external volume"},
		{"volume plugin", `"volumes":{"h":{"driver":"rexray"}}`, "volume driver"},
		{"external network", `"networks":{"n":{"external":true,"name":"shared"}}`, "external network"},
		{"host network by name", `"networks":{"n":{"name":"host"}}`, "network"},
		{"macvlan", `"networks":{"n":{"driver":"macvlan"}}`, "network"},
		{"a network that is another project's", `"networks":{"default":{"name":"other_default"}}`, "network name outside the project"},
		{"a volume that is another project's", `"volumes":{"data":{"name":"other_data"}}`, "volume name outside the project"},
		{"network ipam", `"networks":{"default":{"name":"ns_default","ipam":{"driver":"foo"}}}`, "recipe key not allowed"},
		{"network driver options", `"networks":{"default":{"name":"ns_default","driver_opts":{"k":"v"}}}`, "recipe key not allowed"},
		{"models", `"models":{"m":{"model":"x"}}`, "recipe key not allowed"},
		{"secret from the host", `"secrets":{"s":{"file":"/etc/shadow"}}`, "secret file outside the checkout"},
		{"config from the host", `"configs":{"c":{"file":"/root/.ssh/id_rsa"}}`, "config file outside the checkout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := `{"services":{"web":{"image":"x"}},` + tt.top + `}`
			got, err := Lint([]byte(doc), checkout, "ns")
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range got {
				if v.Rule == tt.rule {
					return
				}
			}
			t.Errorf("violations = %v, want rule %q", got, tt.rule)
		})
	}
	ok := `{"services":{"web":{"image":"x"}},"volumes":{"data":{}},"networks":{"default":{"name":"ns_default","driver":"bridge"}},"secrets":{"s":{"file":"/work/example-solo/s.txt"}}}`
	if got, err := Lint([]byte(ok), checkout, "ns"); err != nil || len(got) != 0 {
		t.Errorf("a plain top level: %v, %v", got, err)
	}
}

// The code under test commits the checkout: a symlink in it must not turn the host into
// "inside the checkout".
func TestLintFollowsSymlinksOutOfTheCheckout(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "host")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/var/run/docker.sock", filepath.Join(root, "sock")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ok"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{filepath.Join(root, "host"), filepath.Join(root, "host", "deeper"), filepath.Join(root, "sock")} {
		svc := `{"image":"x","volumes":[{"type":"bind","source":"` + src + `","target":"/h"}]}`
		got, err := Lint(recipe(svc), root, "ns")
		if err != nil || len(got) == 0 {
			t.Errorf("source %s passed: %v, %v", src, got, err)
		}
	}
	svc := `{"image":"x","volumes":[{"type":"bind","source":"` + filepath.Join(root, "ok") + `","target":"/h"},{"type":"bind","source":"` + filepath.Join(root, "new-dir") + `","target":"/n"}]}`
	if got, err := Lint(recipe(svc), root, "ns"); err != nil || len(got) != 0 {
		t.Errorf("a real directory and a not yet created one inside the checkout: %v, %v", got, err)
	}
}

// A name that contains two dots is refused, because it could be a link that the checks cannot
// follow safely; the path then counts as outside the checkout.
func TestAPathWithTwoDotsInANameIsOutside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if inside(root, filepath.Join(root, "a..b", "c")) {
		t.Error("a path with a name that contains two dots was accepted")
	}
	if !inside(root, filepath.Join(root, "ab", "c")) {
		t.Error("an ordinary path inside the checkout was refused")
	}
}

// The project's own annotations and the keys the lint knows stay allowed.
func TestLintAllowsAnnotationsAndKnownKeys(t *testing.T) {
	t.Parallel()
	svc := `{"image":"x","x-owner":"team","command":["run"],"environment":{"A":"b"},"healthcheck":{"test":["CMD","true"]},"depends_on":{"db":{"condition":"service_healthy"}},"cpus":0.5,"mem_limit":"128m","build":{"context":"/work/example-solo","args":{"A":"b"},"target":"dev"}}`
	got, err := Lint(recipe(svc), checkout, "ns")
	if err != nil || len(got) != 0 {
		t.Fatalf("violations = %v, %v; want none", got, err)
	}
}

// An env file is read by the engine at run time. With the file's content folded into the
// environment the lint could not see where it came from, so the lint wants the path.
func TestLintRefusesAnEnvFileThatLeavesTheCheckoutByALink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.env")
	if err := os.WriteFile(secret, []byte("K=v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "app.env")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	got, err := Lint(recipe(`{"image":"x","env_file":[{"path":"`+link+`"}]}`), root, "ns")
	if err != nil || len(got) == 0 {
		t.Fatalf("a link to a file outside the checkout passed: %v, %v", got, err)
	}
}

func TestLintAllowsNetworkModeNoneAndASiblingService(t *testing.T) {
	t.Parallel()
	doc := `{"services":{"a":{"image":"x","network_mode":"none"},"b":{"image":"x"},"c":{"image":"x","network_mode":"service:b"}}}`
	if got, err := Lint([]byte(doc), checkout, "ns"); err != nil || len(got) != 0 {
		t.Fatalf("violations = %v, %v; want none", got, err)
	}
}
