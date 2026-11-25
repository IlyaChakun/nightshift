package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// upgradeRepo is a repository with the upgrade script, stand-ins for the scripts it calls and
// for docker and gh, and a log of what was called.
type upgradeRepo struct {
	t         *testing.T
	dir, bin  string
	log       string
	drillExit string
	state     string // the state the sandbox task reaches
}

func newUpgradeRepo(t *testing.T) *upgradeRepo {
	t.Helper()
	r := &upgradeRepo{t: t, dir: t.TempDir(), bin: t.TempDir(), drillExit: "0", state: "released"}
	r.log = filepath.Join(t.TempDir(), "calls")
	script, err := os.ReadFile("upgrade.sh")
	if err != nil {
		t.Fatal(err)
	}
	r.write(r.dir, "deploy/upgrade.sh", string(script))
	r.write(r.dir, "deploy/config/instance.yaml", "name: t\n")
	r.write(r.dir, "deploy/.env", "")
	r.write(r.dir, "deploy/secrets/token", "x")
	r.write(r.dir, "scripts/drill.sh", "#!/bin/sh\necho drill >>\"$CALLS\"\nexit \"$DRILL_EXIT\"\n")
	r.write(r.dir, "scripts/stable.sh", "#!/bin/sh\necho \"stable $*\" >>\"$CALLS\"\ncase \"$*\" in\ncurrent) echo old-tag ;;\n*psql*) echo 0 ;;\nesac\n")
	r.write(r.bin, "gh", "#!/bin/sh\necho 'completed success'\n")
	r.write(r.bin, "docker", `#!/bin/sh
echo "docker $*" >>"$CALLS"
case "$*" in
*"task add"*) echo 'queued task #7 in example/app: x' ;;
*"SELECT state"*) echo "$TASK_STATE" ;;
esac
`)
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "scripts", "deploy/upgrade.sh"},
		{"-c", "user.name=t", "-c", "user.email=t@example.test", "commit", "-q", "-m", "init"},
		{"remote", "add", "origin", "."},
		{"fetch", "-q", "origin"},
	} {
		r.git(args...)
	}
	return r
}

func (r *upgradeRepo) write(root, name, body string) {
	r.t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil { //nolint:gosec // G306: the test runs these scripts
		r.t.Fatal(err)
	}
}

func (r *upgradeRepo) git(args ...string) string {
	r.t.Helper()
	out, err := exec.CommandContext(r.t.Context(), "git", append([]string{"-C", r.dir}, args...)...).CombinedOutput() //nolint:gosec // fixed arguments in a temporary repository
	if err != nil {
		r.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func (r *upgradeRepo) run(args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.CommandContext(r.t.Context(), "bash", append([]string{filepath.Join(r.dir, "deploy/upgrade.sh")}, args...)...) //nolint:gosec // the script under test
	cmd.Env = append(os.Environ(), "PATH="+r.bin+":"+os.Getenv("PATH"), "CALLS="+r.log,                                        //nolint:forbidigo // the script needs the test's PATH with the stand-ins first
		"DRILL_EXIT="+r.drillExit, "TASK_STATE="+r.state, "NIGHTSHIFT_CHECK_POLL_SECONDS=0", "NIGHTSHIFT_CHECK_SETTLE_SECONDS=0")
	out, err := cmd.CombinedOutput()
	calls, _ := os.ReadFile(r.log)
	return string(out), string(calls), err
}

func TestUpgradeChecksOnlyAndChangesNothing(t *testing.T) {
	t.Parallel()
	r := newUpgradeRepo(t)
	out, calls, err := r.run("--check-only")
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	// The test repository pins no tools, so there is no tool install.
	for _, want := range []string{"drill", "stable compose stop worker", "restore-test.sh", "offsite.sh", "modcache", "stable compose start worker"} {
		if !strings.Contains(calls, want) {
			t.Errorf("no call with %q:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "stable up") || strings.TrimSpace(r.git("tag")) != "" {
		t.Errorf("a check changed the instance:\n%s", calls)
	}
}

func TestAFailedCheckTagsNothingAndWakesTheStableWorker(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		drill, state string
		paused       bool
	}{
		"drill":        {drill: "1", state: "released"},
		"sandbox task": {drill: "0", state: "escalated", paused: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newUpgradeRepo(t)
			r.drillExit, r.state = tc.drill, tc.state
			out, calls, err := r.run()
			if err == nil {
				t.Fatalf("passed: %s", out)
			}
			if strings.Contains(calls, "stable up") || strings.TrimSpace(r.git("tag")) != "" {
				t.Errorf("a failed check upgraded:\n%s", calls)
			}
			if got := strings.Contains(calls, "stable compose start worker"); got != tc.paused {
				t.Errorf("worker started again: %v, want %v\n%s", got, tc.paused, calls)
			}
		})
	}
}

func TestUpgradeTagsAndStartsTheNewRelease(t *testing.T) {
	t.Parallel()
	r := newUpgradeRepo(t)
	out, calls, err := r.run()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	tag := strings.TrimSpace(r.git("tag"))
	if !strings.HasPrefix(tag, "factory-stable-") || !strings.Contains(calls, "stable up "+tag) {
		t.Errorf("tag %q:\n%s", tag, calls)
	}
}
