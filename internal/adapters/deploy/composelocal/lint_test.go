package composelocal

import (
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
		{"unknown mount type", `{"image":"x","volumes":[{"type":"npipe","source":"x","target":"/k"}]}`, "mount type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Lint(recipe(tt.svc), checkout)
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
	got, err := Lint(recipe(svc), checkout)
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
		if v, err := Lint([]byte(doc), checkout); err == nil {
			t.Errorf("%s: Lint accepted it with violations %v", name, v)
		}
	}
}

func TestLintNamesTheServiceAndReportsEveryProblem(t *testing.T) {
	t.Parallel()
	got, _ := Lint([]byte(`{"services":{"a":{"privileged":true},"b":{"network_mode":"host","cap_add":["NET_ADMIN"]}}}`), checkout)
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
