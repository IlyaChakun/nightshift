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
	// The acceptance Verifier browses through a pinned Playwright MCP server (owner decision
	// 2026-11-28); cmd/nightshift/work.go names the path it installs to.
	for _, want := range []string{"--prefix /opt/playwright-mcp", "@playwright/mcp@${PLAYWRIGHT_MCP_VERSION}", "/opt/playwright-mcp/bin/playwright-mcp --version"} {
		if !strings.Contains(worker, want) {
			t.Errorf("the worker stage lacks %q", want)
		}
	}
	if !regexp.MustCompile(`ARG PLAYWRIGHT_MCP_VERSION=\d+\.\d+\.\d+\n`).MatchString(worker) {
		t.Error("PLAYWRIGHT_MCP_VERSION must default to an exact version")
	}
	work, err := os.ReadFile("../cmd/nightshift/work.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(work), `"/opt/playwright-mcp/bin/playwright-mcp"`) {
		t.Error("the worker does not start the browser tool the image installs")
	}
	skill, err := os.ReadFile("../skills/own/browser-acceptance/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	// An acceptance job has no shell: the skill uses the browser tool, never node or a script.
	for _, want := range []string{"mcp__browser__", "browser_take_screenshot", "browser_navigate"} {
		if !strings.Contains(string(skill), want) {
			t.Errorf("the skill must name %q", want)
		}
	}
	for _, bad := range []string{"require(", "run them with `node`", "npx "} {
		if strings.Contains(string(skill), bad) {
			t.Errorf("the skill still tells the agent to run code: %q", bad)
		}
	}
}
