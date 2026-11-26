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
	for _, want := range []string{"chromium", "playwright-core@${PLAYWRIGHT_VERSION}", "NIGHTSHIFT_CHROMIUM=/usr/bin/chromium"} {
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
	if !strings.Contains(string(skill), "$NIGHTSHIFT_CHROMIUM") || !strings.Contains(string(skill), "--no-sandbox") {
		t.Error("the skill must launch the image's Chromium without its own sandbox")
	}
}
