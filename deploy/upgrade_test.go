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
	upExit    string // exit status of starting a new release
	services  string // what the stable instance lists after the upgrade
	state     string // the state the sandbox task reaches
	ciOK      string // what gh says about the ci-ok check; empty: completed success
	verify    string // what gh says about the verify runs of the commit
}

func newUpgradeRepo(t *testing.T) *upgradeRepo {
	t.Helper()
	r := &upgradeRepo{t: t, dir: t.TempDir(), bin: t.TempDir(), drillExit: "0", upExit: "0", state: "released"}
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
	r.write(r.dir, "scripts/stable.sh", "#!/bin/sh\necho \"stable $*\" >>\"$CALLS\"\ncase \"$*\" in\ncurrent) echo old-tag ;;\n*psql*) echo 0 ;;\n*\" ps \"*) printf '%s\\n' \"${SERVICES:-migrate exited}\" ;;\n\"up old-tag\") exit 0 ;;\nup\\ *) exit \"${UP_EXIT:-0}\" ;;\nesac\n")
	// gh answers the check-runs query with $CI_OK and the workflow-runs query with $VERIFY_RUNS.
	r.write(r.bin, "gh", "#!/bin/sh\ncase \"$*\" in\n*actions/runs*) echo \"$VERIFY_RUNS\" ;;\n*) echo \"${CI_OK:-completed success}\" ;;\nesac\n")
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
	return r.runWith("", args...)
}

// runWith runs the script with one more environment entry (empty: none).
func (r *upgradeRepo) runWith(env string, args ...string) (string, string, error) {
	r.t.Helper()
	cmd := exec.CommandContext(r.t.Context(), "bash", append([]string{filepath.Join(r.dir, "deploy/upgrade.sh")}, args...)...)                                                         //nolint:gosec // the script under test
	cmd.Env = append(os.Environ(), "NIGHTSHIFT_STABLE_HOME="+r.bin, "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test", "PATH="+r.bin+":"+os.Getenv("PATH"), "CALLS="+r.log, //nolint:forbidigo // the script needs the test's PATH with the stand-ins first
		"DRILL_EXIT="+r.drillExit, "UP_EXIT="+r.upExit, "SERVICES="+r.services, "TASK_STATE="+r.state, "CI_OK="+r.ciOK, "VERIFY_RUNS="+r.verify, "NIGHTSHIFT_CHECK_POLL_SECONDS=0", "NIGHTSHIFT_CHECK_SETTLE_SECONDS=0")
	if env != "" {
		cmd.Env = append(cmd.Env, env)
	}
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
	up := strings.Index(calls, "stable up "+tag)
	if !strings.HasPrefix(tag, "factory-stable-") || up < 0 {
		t.Fatalf("tag %q:\n%s", tag, calls)
	}
	// One subscription session at a time: the development stack is gone before the stable
	// worker starts again.
	if down := strings.Index(calls, " down"); down < 0 || down > up {
		t.Errorf("the development stack was not stopped before the upgrade:\n%s", calls)
	}
}

func TestAReleaseThatDoesNotStartIsRolledBack(t *testing.T) {
	t.Parallel()
	r := newUpgradeRepo(t)
	r.upExit = "1"
	out, calls, err := r.run()
	if err == nil || !strings.Contains(calls, "stable up old-tag") || !strings.Contains(out, "old-tag is running again") {
		t.Fatalf("%v: %s\n%s", err, out, calls)
	}
}

// A file that pins no agent CLI version must not end the run without a word.
func TestEnvFilesWithoutVersionsAreFine(t *testing.T) {
	t.Parallel()
	r := newUpgradeRepo(t)
	r.write(r.bin, "env", "NIGHTSHIFT_TELEMETRY=0\n")
	if out, _, err := r.run("--check-only"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestAStoppedServiceAfterTheUpgradeIsReported(t *testing.T) {
	t.Parallel()
	r := newUpgradeRepo(t)
	r.services = "worker exited"
	out, _, err := r.run()
	if err == nil || !strings.Contains(out, "does not stay up") {
		t.Fatalf("%v: %s", err, out)
	}
}

// A queued verify run replaced by a newer push is canceled and never creates ci-ok. The script
// must say so at once (a real run waited the whole ci timeout), and must keep waiting while a
// run of the commit is still alive.
func TestUpgradeStopsAtOnceWhenTheVerifyRunWasCancelled(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		verify   string
		wantFast bool
	}{
		"canceled":                      {verify: "completed cancelled", wantFast: true}, //nolint:misspell // GitHub's conclusion value
		"canceled, then a rerun queued": {verify: "completed cancelled,queued null"},     //nolint:misspell // GitHub's conclusion value
		"no run yet":                    {verify: ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newUpgradeRepo(t)
			r.ciOK, r.verify = "null null", tc.verify
			out, calls, err := r.runWith("NIGHTSHIFT_CHECK_CI_SECONDS=0")
			if err == nil || strings.Contains(calls, "drill") || strings.Contains(calls, "stable up") {
				t.Fatalf("went on without ci-ok: %v\n%s\n%s", err, out, calls)
			}
			if got := strings.Contains(out, "before ci-ok ran"); got != tc.wantFast {
				t.Errorf("says canceled: %v, want %v\n%s", got, tc.wantFast, out)
			}
		})
	}
}
