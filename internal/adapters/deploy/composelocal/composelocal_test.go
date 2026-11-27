package composelocal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// fakeExec answers docker calls from a script keyed by the compose verb and records every call.
type fakeExec struct {
	rendered string            // answer of `compose config`
	psOut    map[string]string // answer by "compose" or "docker" ps
	fail     map[string]error  // by verb
	calls    []string
	envs     [][]string
	dirs     []string
	onUp     func(args []string)
}

func (f *fakeExec) Run(_ context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.envs = append(f.envs, env)
	f.dirs = append(f.dirs, dir)
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

type tree struct {
	dir      string
	recipe   string // the recipe file; empty means a plain one-service file
	rendered map[string][]byte
}

// Render keeps the rewritten recipes in memory, by project.
func (t tree) Render(proj string, data []byte) (string, error) {
	t.rendered[proj] = data
	return "/render/" + proj + ".json", nil
}

func (t tree) Checkout(_ context.Context, _ deployapi.EnvRef, _ deployapi.Revision) (Tree, func(), error) {
	return Tree{Dir: t.dir, Recipe: t.recipe, Main: "/work/main"}, func() {}, nil
}

const okRecipe = `{"services":{"web":{"image":"x","ports":[{"target":80,"published":"0","host_ip":"0.0.0.0"}]}}}`

func newDeployer(t *testing.T, f *fakeExec) *Deployer {
	t.Helper()
	return deployerIn(t, f, "services:\n  web:\n    image: x\n")
}

// deployerIn gives a deployer whose checkout is a real directory with this recipe file in it.
func deployerIn(t *testing.T, f *fakeExec, recipe string) *Deployer {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte(recipe), 0o600); err != nil {
		t.Fatal(err)
	}
	return &Deployer{
		Cfg:       Config{Tenant: "example-solo", Entry: "web", CPUs: 2, Memory: "2g", Smoke: "./smoke.sh"},
		Checkouts: tree{dir: dir, recipe: file, rendered: map[string][]byte{}}, Exec: f, Work: t.TempDir(),
	}
}

func TestUpRewritesTheRecipeAndReturnsTheEndpoint(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"compose": `{"State":"running","Publishers":[{"PublishedPort":49153}]}`}}
	d := newDeployer(t, f)
	ep, err := d.Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc")
	if err != nil || ep.URL != "http://127.0.0.1:49153" {
		t.Fatalf("Up = %+v, %v", ep, err)
	}
	got := renderedFile(d)
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
	_, err := newDeployer(t, f).Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc")
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
	if _, err := newDeployer(t, f).Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err == nil || !strings.Contains(err.Error(), "publishes no port") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvironmentNamesAreSafe(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe}
	for _, e := range []deployapi.EnvRef{{Tenant: "Example", Task: "1"}, {Tenant: "a", Task: "../x"}, {Tenant: "a b", Task: "1"}, {Tenant: "a", Task: "1;rm"}} {
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
	res, err := newDeployer(t, f).Smoke(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"})
	if err != nil || !res.OK {
		t.Fatalf("Smoke = %+v, %v", res, err)
	}
	if last := f.envs[len(f.envs)-1]; len(last) != 1 || last[0] != "NS_URL=http://127.0.0.1:8080" {
		t.Errorf("the check got env %v", last)
	}
	if f.dirs[len(f.dirs)-1] != "/work/main" {
		t.Errorf("the smoke check ran in %q; it must run in the checkout of main", f.dirs[len(f.dirs)-1])
	}

	f.fail = map[string]error{"sh": errors.New("exit 1")}
	if res, err = newDeployer(t, f).Smoke(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}); err != nil || res.OK || res.Detail != "boom output" {
		t.Errorf("failing check: %+v, %v", res, err)
	}

	f.psOut["docker"] = `{"State":"running","Status":"Up (unhealthy)"}`
	if res, _ = newDeployer(t, f).Smoke(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}); res.OK || !strings.Contains(res.Detail, "unhealthy") {
		t.Errorf("unhealthy stack: %+v", res)
	}
}

func TestDownOfProductionIsRefusedAndOfAPreviewRemovesVolumes(t *testing.T) {
	t.Parallel()
	f := &fakeExec{}
	d := newDeployer(t, f)
	if err := d.Down(t.Context(), deployapi.EnvRef{Tenant: "example-solo"}); !errors.Is(err, deployapi.ErrUnsupported) {
		t.Errorf("Down(production) = %v", err)
	}
	if err := d.Down(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}); err != nil {
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
		want deployapi.EnvState
	}{
		{"nothing", ``, deployapi.EnvAbsent},
		{"healthy", `{"State":"running","Status":"Up 3 minutes (healthy)"}`, deployapi.EnvHealthy},
		{"starting", `{"State":"running","Status":"Up 3 seconds (health: starting)"}`, deployapi.EnvStarting},
		{"unhealthy beats starting", "{\"State\":\"running\",\"Status\":\"Up (health: starting)\"}\n{\"State\":\"running\",\"Status\":\"Up (unhealthy)\"}", deployapi.EnvUnhealthy},
		{"crashed", `{"State":"exited","Status":"Exited (1) 2 seconds ago"}`, deployapi.EnvUnhealthy},
		{"one-shot job finished", "{\"State\":\"exited\",\"Status\":\"Exited (0) 1 minute ago\"}\n{\"State\":\"running\",\"Status\":\"Up (healthy)\"}", deployapi.EnvHealthy},
		{"array form", `[{"State":"running","Status":"Up (healthy)"}]`, deployapi.EnvHealthy},
	}
	for _, tt := range tests {
		if got := statusOf(parsePS([]byte(tt.out))).State; got != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestReleaseOfAHalfUpgradedStackKeepsTheLastGoodRevision(t *testing.T) {
	t.Parallel()
	// A is live; Release(B) failed midway and left B unhealthy, with A as its previous.
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{
		"docker":  `{"State":"running","Status":"Up (unhealthy)","Labels":"ns.revision=B,ns.previous=A"}`,
		"compose": `{"Publishers":[{"PublishedPort":8080}]}`,
	}}
	res, err := newDeployer(t, f).Release(t.Context(), "C")
	if err != nil || res.Previous != "A" {
		t.Fatalf("Release = %+v, %v; the broken B must not become the way back", res, err)
	}
}

func TestRewriteFailsClosedOnAPortItCannotRead(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: `{"services":{"web":{"image":"x","ports":["8080:80"]}}}`}
	if _, err := newDeployer(t, f).Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("err = %v", err)
	}
}

func TestTheProjectsOwnLimitsStay(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: `{"services":{"web":{"image":"x","mem_limit":"256m","ports":[{"target":80}]}}}`, psOut: map[string]string{"compose": `{"Publishers":[{"PublishedPort":1}]}`}}
	d := newDeployer(t, f)
	if _, err := d.Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err != nil {
		t.Fatal(err)
	}
	got := renderedFile(d)
	if !strings.Contains(string(got), `"mem_limit":"256m"`) || strings.Contains(string(got), `"mem_limit":"2g"`) {
		t.Errorf("limits:\n%s", got)
	}
}

func TestEntryMustBeAServiceName(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe}
	d := newDeployer(t, f)
	d.Cfg.Entry = "--all"
	if _, err := d.Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err == nil {
		t.Error("an entry that looks like a flag was accepted")
	}
}

func TestOnlyASmallEnvironmentReachesDocker(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	t.Setenv("DOCKER_HOST", "unix:///x.sock")
	env := strings.Join(passEnv(), "\n")
	if strings.Contains(env, "ANTHROPIC_API_KEY") || !strings.Contains(env, "DOCKER_HOST=") {
		t.Errorf("environment:\n%s", env)
	}
}

// renderedFile is the rewritten recipe that the adapter stored for the preview of task 7.
func renderedFile(d *Deployer) []byte {
	return d.Checkouts.(tree).rendered["ns-example-solo-7"]
}

// The engine reads env files itself; asking `config` to keep them as paths lets the lint see
// where each one points.
func TestRenderKeepsEnvFilesAsPaths(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"compose": `{"Publishers":[{"PublishedPort":1}]}`}}
	if _, err := newDeployer(t, f).Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.calls[0], "config --no-env-resolution --format json") {
		t.Errorf("render call = %q", f.calls[0])
	}
}

// A service that sets no limit gets one: decision 13 wants a ceiling on every preview.
func TestAServiceWithoutLimitsGetsTheDefaults(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: `{"services":{"web":{"image":"x","ports":[{"target":80}]}}}`, psOut: map[string]string{"compose": `{"Publishers":[{"PublishedPort":1}]}`}}
	d := newDeployer(t, f)
	d.Cfg.CPUs, d.Cfg.Memory = 0, ""
	if _, err := d.Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err != nil {
		t.Fatal(err)
	}
	got := string(renderedFile(d))
	for _, want := range []string{`"cpus":1`, `"mem_limit":"1g"`, `"pids_limit":256`} {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten recipe lacks %s:\n%s", want, got)
		}
	}
}

// What the engine printed on a failure reaches the error, so a refused build is not a bare status.
func TestAFailedComposeCallKeepsTheEnginesMessage(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe, fail: map[string]error{"up": errors.New("exit status 1")}}
	_, err := newDeployer(t, f).Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc")
	if err == nil || !strings.Contains(err.Error(), "boom output") {
		t.Fatalf("err = %v, want the output of the failed call", err)
	}
}

// A limit that is null counts as not set, so it gets the default like an absent one.
func TestANullLimitGetsTheDefault(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: `{"services":{"web":{"image":"x","mem_limit":null,"cpus":null,"ports":[{"target":80}]}}}`, psOut: map[string]string{"compose": `{"Publishers":[{"PublishedPort":1}]}`}}
	d := newDeployer(t, f)
	if _, err := d.Up(t.Context(), deployapi.EnvRef{Tenant: "example-solo", Task: "7"}, "abc"); err != nil {
		t.Fatal(err)
	}
	if got := string(renderedFile(d)); !strings.Contains(got, `"mem_limit":"2g"`) || strings.Contains(got, `"mem_limit":null`) {
		t.Errorf("limits:\n%s", got)
	}
}

// A revision whose stack does not come up is the revision's fault: Release says so and still
// reports the revision to go back to, so the conductor can roll back. A platform error stays plain.
func TestReleaseThatDoesNotComeUpReportsThePreviousRevision(t *testing.T) {
	t.Parallel()
	const live = `{"State":"running","Status":"Up 2 hours (healthy)","Labels":"ns.revision=old"}`
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"docker": live}, fail: map[string]error{"up": errors.New("exit status 1")}}
	res, err := newDeployer(t, f).Release(t.Context(), "new")
	if !errors.Is(err, deployapi.ErrRolloutFailed) || res.Previous != "old" || res.Revision != "new" {
		t.Fatalf("Release = %+v, %v; want ErrRolloutFailed and Previous old", res, err)
	}
	f = &fakeExec{rendered: okRecipe, psOut: map[string]string{"docker": live}, fail: map[string]error{"config": errors.New("exit status 1")}}
	if _, err := newDeployer(t, f).Release(t.Context(), "new"); err == nil || errors.Is(err, deployapi.ErrRolloutFailed) {
		t.Fatalf("a render failure is the platform's: %v", err)
	}
}
