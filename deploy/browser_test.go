package deploy_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestWorkerImageCarriesTheBrowser (T6, static): the Verifier's UI acceptance needs a browser, and
// the worker has no way to download one (design section 12), so the image must hold Chromium and a
// pinned Playwright library, and the skill must not tell the agent to install anything.
func TestWorkerImageCarriesTheBrowser(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	_, worker, ok := strings.Cut(string(raw), "AS worker")
	if !ok {
		t.Fatal("no worker stage in the Dockerfile")
	}
	for _, want := range []string{"chromium", "--prefix /opt/playwright", "playwright-core@${PLAYWRIGHT_VERSION}", "chromium --version"} {
		if !strings.Contains(worker, want) {
			t.Errorf("the worker stage lacks %q", want)
		}
	}
	if !regexp.MustCompile(`ARG PLAYWRIGHT_VERSION=\d+\.\d+\.\d+`).MatchString(worker) {
		t.Error("PLAYWRIGHT_VERSION must default to an exact version")
	}
	skill, err := os.ReadFile("../skills/own/browser-acceptance/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	// The agent's environment is an allowlist (harness Env), so the skill must not rely on NODE_PATH
	// or any image ENV: it names the library and the browser by path (review finding).
	for _, want := range []string{"/opt/playwright/lib/node_modules/playwright-core", "/usr/bin/chromium", "--no-sandbox"} {
		if !strings.Contains(string(skill), want) {
			t.Errorf("the skill must name %q", want)
		}
	}
	// A preview is plain http behind egress-preview, which refuses everything else; https goes
	// to the egress proxy (V2 review, 2026-11-28), so the browser names one proxy per scheme.
	for _, want := range []string{"`http=${HTTP_PROXY || http_proxy}`", "`https=${HTTPS_PROXY}`", "--proxy-server=", "--proxy-bypass-list="} {
		if !strings.Contains(string(skill), want) {
			t.Errorf("the skill must launch Chromium with %s", want)
		}
	}
	if strings.Contains(string(skill), "NODE_PATH`; ") || strings.Contains(string(skill), "process.env.NIGHTSHIFT") {
		t.Error("the skill must not depend on environment variables the agent does not receive")
	}
}
