package runner

import (
	"cmp"
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
// the turn writes, owned by the agent user, and the browser configuration beside it in the job
// directory. A preview is plain http behind the preview proxy and https goes to the egress
// proxy, so Chromium gets one proxy per scheme from the job's environment.
func (r *Runner) prepareBrowser(jobDir string, env map[string]string) (*harness.Browser, error) {
	evidence := filepath.Join(jobDir, "evidence")
	if err := os.Mkdir(evidence, 0o700); err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	if r.AgentUID != 0 {
		if err := os.Chown(evidence, r.AgentUID, r.AgentGID); err != nil {
			return nil, fmt.Errorf("runner: %w", err)
		}
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
		args = append(args, "--proxy-server="+strings.Join(servers, ";"), "--proxy-bypass-list="+bypassList(cmp.Or(env["NO_PROXY"], env["no_proxy"])))
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

// bypassList is NO_PROXY in Chromium's form: semicolons, an IPv6 address in brackets.
func bypassList(noProxy string) string {
	var out []string
	for h := range strings.SplitSeq(noProxy, ",") {
		h = strings.TrimSpace(h)
		switch {
		case h == "":
		case strings.Contains(h, "::"):
			out = append(out, "["+h+"]")
		default:
			out = append(out, h)
		}
	}
	return strings.Join(out, ";")
}

// browserFor gives an acceptance job its browser; any other job loses the preview proxy.
func (r *Runner) browserFor(job jobsapi.Job, jobDir string, env map[string]string) (*harness.Browser, error) {
	if !acceptanceJob(job) {
		dropPreviewProxy(env)
		return nil, nil //nolint:nilnil // a job without a browser is not an error
	}
	return r.prepareBrowser(jobDir, env)
}
