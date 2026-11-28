package runner

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// defaultChromium is the system Chromium of the worker image.
const defaultChromium = "/usr/bin/chromium"

// chromiumFlags keep Chromium to the page: no update, clock or other background calls, which the
// proxies refuse (V2 on the Mac, 2026-11-28), and no shared memory the container lacks.
var chromiumFlags = []string{
	"--disable-dev-shm-usage", "--disable-background-networking", "--disable-component-update",
	"--disable-features=NetworkTimeServiceQuerying",
}

// prepareBrowser gives an acceptance job its browser tool: an evidence directory, the only place
// the turn writes, and the browser configuration beside it in the job directory. A preview is
// plain http behind the preview proxy and https goes to the egress proxy, so Chromium gets one
// proxy per scheme from the job's environment.
//
// The worker holds only SETUID and SETGID (deploy/docker-compose.yml): it cannot chown, nor read
// past a mode. So the runner keeps the directory and opens it to all (0777, no sticky bit): the
// agent user writes its screenshots there, the server's umask 022 leaves them readable, and the
// runner reads them and clears them as the agent user (clearEvidence). The job directory
// belongs to this job alone, and the agent user is the only other user in the worker.
func (r *Runner) prepareBrowser(jobDir string, env map[string]string) (*harness.Browser, error) {
	evidence := filepath.Join(jobDir, "evidence")
	if err := os.Mkdir(evidence, 0o700); err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	if err := os.Chmod(evidence, 0o777); err != nil { //nolint:gosec // the agent user writes the screenshots; see above
		return nil, fmt.Errorf("runner: %w", err)
	}
	args := append([]string{}, chromiumFlags...)
	var servers []string
	if p := cmp.Or(env["HTTP_PROXY"], env["http_proxy"]); p != "" {
		servers = append(servers, "http="+p)
	}
	if p := env["HTTPS_PROXY"]; p != "" {
		servers = append(servers, "https="+p)
	}
	if len(servers) > 0 {
		args = append(args, "--proxy-server="+strings.Join(servers, ";"), "--proxy-bypass-list="+loopbackOnly)
	}
	chromium := r.Chromium
	if chromium == "" {
		chromium = defaultChromium
	}
	cfg := map[string]any{
		"browser": map[string]any{
			"browserName": "chromium",
			"isolated":    true,
			"launchOptions": map[string]any{
				"executablePath": chromium, "headless": true, "chromiumSandbox": false, "args": args,
			},
			"contextOptions": map[string]any{"viewport": map[string]int{"width": 1280, "height": 800}},
		},
		"outputDir": evidence,
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	path := filepath.Join(jobDir, "browser.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil { //nolint:gosec // the agent's browser server reads it; it holds no secrets
		return nil, fmt.Errorf("runner: %w", err)
	}
	return &harness.Browser{EvidenceDir: evidence, Config: path}, nil
}

// loopbackOnly is the browser's whole bypass list: every other host, the factory's own services
// included, goes through a proxy, though the worker's NO_PROXY names those services.
const loopbackOnly = "localhost;127.0.0.1;[::1]"

// browserFor gives an acceptance job its browser when the harness wires one (harness.Browsing);
// on another harness the job keeps its screenshots in the checkout. Any job that is not an
// acceptance job loses the preview proxy.
func (r *Runner) browserFor(h harness.Harness, job jobsapi.Job, jobDir string, env map[string]string) (*harness.Browser, error) {
	if !acceptanceJob(job) {
		dropPreviewProxy(env)
		return nil, nil //nolint:nilnil // a job without a browser is not an error
	}
	if b, ok := h.(harness.Browsing); !ok || !b.Browses() {
		return nil, nil //nolint:nilnil // a harness without a browser tool browses with its shell
	}
	return r.prepareBrowser(jobDir, env)
}

// clearEvidence removes what the agent user left in the evidence directory, as that user: the
// runner may not unlink inside a directory the agent made there (a screenshot name with a
// slash), and the job directory would stay behind.
func (r *Runner) clearEvidence(ctx context.Context, b *harness.Browser) {
	if b == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DeliverTimeout)
	defer cancel()
	if err := r.asAgent(ctx, "find", b.EvidenceDir, "-mindepth", "1", "-delete"); err != nil {
		r.log().WarnContext(ctx, "evidence not cleared", "dir", b.EvidenceDir, "error", err)
	}
}

// dropJobDir removes the job directory, and says so when it cannot.
func (r *Runner) dropJobDir(ctx context.Context, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		r.log().WarnContext(ctx, "job directory not removed", "dir", dir, "error", err)
	}
}
