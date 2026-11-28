package runner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// capsChild marks the copy of the test binary that runs with the worker's capabilities.
const capsChild = "NIGHTSHIFT_TEST_WORKER_CAPS"

// T6: an acceptance job's evidence directory works with the worker's capabilities, SETUID and
// SETGID only (deploy/docker-compose.yml): the runner prepares it, the agent user writes a
// screenshot, the runner keeps it and removes the job directory. V4 on the Mac, 2026-11-28: a
// chown of the directory failed and every acceptance job died. The test re-runs itself under
// setpriv with that bounding set, so it needs root and setpriv; CI runs it where it has them.
func TestTheEvidenceDirectoryWorksWithTheWorkersCapabilities(t *testing.T) {
	if os.Getenv(capsChild) == "1" { //nolint:forbidigo // a test marker, not a setting
		evidenceAsTheWorker(t)
		return
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root to drop to the worker's capabilities")
	}
	setpriv, err := exec.LookPath("setpriv")
	if err != nil {
		t.Skip("needs setpriv")
	}
	cmd := exec.CommandContext(t.Context(), setpriv, "--bounding-set", "-all,+setuid,+setgid", "--inh-caps", "-all", //nolint:gosec // the test binary itself
		os.Args[0], "-test.run=^TestTheEvidenceDirectoryWorksWithTheWorkersCapabilities$", "-test.count=1", "-test.v")
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", capsChild + "=1"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("with the worker's capabilities: %v\n%s", err, out)
	}
}

func evidenceAsTheWorker(t *testing.T) {
	t.Helper()
	const agent = 20001
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(probe, agent, agent); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("chown is allowed (%v): not the worker's capability set", err)
	}
	jobDir, err := os.MkdirTemp("", "nightshift-job-") //nolint:usetesting // t.TempDir is private to root, the agent user must reach it as in buildContext
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(jobDir, 0o755); err != nil { //nolint:gosec // as buildContext does
		t.Fatal(err)
	}
	r := &Runner{AgentUID: agent, AgentGID: agent, ArtifactDir: t.TempDir()}
	b, err := r.prepareBrowser(jobDir, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	// The browser server, as the agent user, saves a screenshot with the launcher's umask.
	if err := r.asAgent(t.Context(), "/bin/sh", "-c", `umask 022 && printf '\211PNG\r\n\032\nimage' > "$1/01-start.png"`, "sh", b.EvidenceDir); err != nil {
		t.Fatal(err)
	}
	out, err := r.withEvidence(jobsapi.ClaimResponse{JobID: "1", JobToken: "t"}, b.EvidenceDir, jobsapi.CompleteRequest{})
	if err != nil || len(out.Artifacts) != 1 {
		t.Fatalf("evidence: %+v %v", out.Artifacts, err)
	}
	if err := os.RemoveAll(jobDir); err != nil {
		t.Fatalf("the runner cannot remove the job directory: %v", err)
	}
}
