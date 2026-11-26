package composelocal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// fakeExec answers docker calls from a script keyed by the compose verb and records every call.
type fakeExec struct {
	rendered string            // answer of `compose config`
	psOut    map[string]string // answer by "compose" or "docker" ps
	fail     map[string]error  // by verb
	calls    []string
	envs     [][]string
	onUp     func(args []string)
}

func (f *fakeExec) Run(_ context.Context, _ string, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.envs = append(f.envs, env)
	verb := ""
	for _, a := range args {
		switch a {
		case "config", "up", "down", "ps":
			verb = a
		}
	}
	if name == "sh" {
		verb = "sh"
	}
	if err := f.fail[verb]; err != nil {
		return []byte("boom output"), err
	}
	switch {
	case verb == "config":
		return []byte(f.rendered), nil
	case verb == "up":
		if f.onUp != nil {
			f.onUp(args)
		}
		return nil, nil
	case verb == "ps" && args[0] != "compose":
		return []byte(f.psOut["docker"]), nil
	case verb == "ps":
		return []byte(f.psOut["compose"]), nil
	}
	return nil, nil
}

type tree struct{ dir string }

func (t tree) Checkout(_ context.Context, rev integration.Revision) (Tree, func(), error) {
	return Tree{Dir: t.dir, Recipe: filepath.Join(t.dir, "deploy/compose.yaml")}, func() {}, nil
}

const okRecipe = `{"services":{"web":{"image":"x","ports":[{"target":80,"published":"0","host_ip":"0.0.0.0"}]}}}`

func newDeployer(t *testing.T, f *fakeExec) *Deployer {
	t.Helper()
	return &Deployer{
		Cfg:       Config{Tenant: "example-solo", Entry: "web", CPUs: 2, Memory: "2g", Smoke: "./smoke.sh"},
		Checkouts: tree{dir: "/work/example-solo"}, Exec: f, Work: t.TempDir(),
	}
}

func TestUpRewritesTheRecipeAndReturnsTheEndpoint(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"compose": `{"State":"running","Publishers":[{"PublishedPort":49153}]}`}}
	d := newDeployer(t, f)
	ep, err := d.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, "abc")
	if err != nil || ep.URL != "http://127.0.0.1:49153" {
		t.Fatalf("Up = %+v, %v", ep, err)
	}
	got, err := os.ReadFile(filepath.Join(d.Work, "ns-example-solo-7.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"host_ip":"127.0.0.1"`, `"mem_limit":"2g"`, `"cpus":2`, `"ns.revision":"abc"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("rewritten recipe lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "0.0.0.0") {
		t.Errorf("a port is still open to every interface:\n%s", got)
	}
	if up := f.calls[len(f.calls)-2]; !strings.Contains(up, "-p ns-example-solo-7") || !strings.Contains(up, "up -d --build --wait") {
		t.Errorf("up call = %q", up)
	}
}

func TestUpRefusesAForbiddenRecipeBeforeAnythingStarts(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: `{"services":{"web":{"image":"x","privileged":true}}}`}
	_, err := newDeployer(t, f).Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, "abc")
	var re *RecipeError
	if !errors.As(err, &re) || len(re.Violations) != 1 {
		t.Fatalf("err = %v, want a RecipeError", err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, " up ") {
			t.Errorf("the engine was asked to start the stack: %s", c)
		}
	}
}

func TestUpWithoutAPublishedPortSaysSo(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"compose": `[{"State":"running","Publishers":[]}]`}}
	if _, err := newDeployer(t, f).Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err == nil || !strings.Contains(err.Error(), "publishes no port") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvironmentNamesAreSafe(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe}
	for _, e := range []integration.EnvRef{{Tenant: "Example", Task: "1"}, {Tenant: "a", Task: "../x"}, {Tenant: "a b", Task: "1"}, {Tenant: "a", Task: "1;rm"}} {
		if _, err := newDeployer(t, f).Up(t.Context(), e, "abc"); err == nil {
			t.Errorf("Up accepted %+v", e)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("docker was called for an unsafe name: %v", f.calls)
	}
}

func TestReleaseReportsThePreviousRevisionAndKeepsItOnRetry(t *testing.T) {
	t.Parallel()
	const live = `{"State":"running","Status":"Up 2 hours (healthy)","Labels":"ns.revision=old,x=y"}`
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"docker": live, "compose": `{"Publishers":[{"PublishedPort":8080}]}`}}
	res, err := newDeployer(t, f).Release(t.Context(), "new")
	if err != nil || res.Previous != "old" || res.Revision != "new" {
		t.Fatalf("Release = %+v, %v", res, err)
	}
	// The retry: production already runs "new" and remembers that it replaced "old".
	f.psOut["docker"] = `{"State":"running","Status":"Up 1 minute (healthy)","Labels":"ns.revision=new,ns.previous=old"}`
	res, err = newDeployer(t, f).Release(t.Context(), "new")
	if err != nil || res.Previous != "old" {
		t.Fatalf("retried Release = %+v, %v; want Previous old", res, err)
	}
}

func TestSmokeNeedsAHealthyStackThenRunsTheProjectsCheck(t *testing.T) {
	t.Parallel()
	healthy := `{"State":"running","Status":"Up (healthy)","Labels":"ns.revision=abc"}`
	f := &fakeExec{psOut: map[string]string{"docker": healthy, "compose": `{"Publishers":[{"PublishedPort":8080}]}`}}
	res, err := newDeployer(t, f).Smoke(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"})
	if err != nil || !res.OK {
		t.Fatalf("Smoke = %+v, %v", res, err)
	}
	if last := f.envs[len(f.envs)-1]; len(last) != 1 || last[0] != "NS_URL=http://127.0.0.1:8080" {
		t.Errorf("the check got env %v", last)
	}

	f.fail = map[string]error{"sh": errors.New("exit 1")}
	if res, err = newDeployer(t, f).Smoke(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}); err != nil || res.OK || res.Detail != "boom output" {
		t.Errorf("failing check: %+v, %v", res, err)
	}

	f.psOut["docker"] = `{"State":"running","Status":"Up (unhealthy)"}`
	if res, _ = newDeployer(t, f).Smoke(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}); res.OK || !strings.Contains(res.Detail, "unhealthy") {
		t.Errorf("unhealthy stack: %+v", res)
	}
}

func TestDownOfProductionIsRefusedAndOfAPreviewRemovesVolumes(t *testing.T) {
	t.Parallel()
	f := &fakeExec{}
	d := newDeployer(t, f)
	if err := d.Down(t.Context(), integration.EnvRef{Tenant: "example-solo"}); !errors.Is(err, integration.ErrUnsupported) {
		t.Errorf("Down(production) = %v", err)
	}
	if err := d.Down(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}); err != nil {
		t.Fatal(err)
	}
	if c := f.calls[0]; !strings.Contains(c, "down --volumes --remove-orphans") {
		t.Errorf("down call = %q", c)
	}
}

func TestStatusFoldsContainers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		out  string
		want integration.EnvState
	}{
		{"nothing", ``, integration.EnvAbsent},
		{"healthy", `{"State":"running","Status":"Up 3 minutes (healthy)"}`, integration.EnvHealthy},
		{"starting", `{"State":"running","Status":"Up 3 seconds (health: starting)"}`, integration.EnvStarting},
		{"unhealthy beats starting", "{\"State\":\"running\",\"Status\":\"Up (health: starting)\"}\n{\"State\":\"running\",\"Status\":\"Up (unhealthy)\"}", integration.EnvUnhealthy},
		{"crashed", `{"State":"exited","Status":"Exited (1) 2 seconds ago"}`, integration.EnvUnhealthy},
		{"one-shot job finished", "{\"State\":\"exited\",\"Status\":\"Exited (0) 1 minute ago\"}\n{\"State\":\"running\",\"Status\":\"Up (healthy)\"}", integration.EnvHealthy},
		{"array form", `[{"State":"running","Status":"Up (healthy)"}]`, integration.EnvHealthy},
	}
	for _, tt := range tests {
		if got := statusOf(parsePS([]byte(tt.out))).State; got != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.name, got, tt.want)
		}
	}
}
