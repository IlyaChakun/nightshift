package runner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// An acceptance job gets a browser tool whose evidence directory is the one place it writes, and
// the screenshots saved there reach the job's artifacts (V4 on the Mac, 2026-11-28: the card had
// none, the read-only turn could not write the checkout).
func TestAnAcceptanceJobKeepsTheScreenshotsOfItsBrowser(t *testing.T) {
	t.Parallel()
	var seen *harness.Browser
	var cfg map[string]any
	r, q := fixture(t, browsingAgent{agent{out: `{}`, peek: func(req harness.RunRequest) {
		seen = req.Browser
		if seen != nil {
			cfg = browseLikeAnAgent(t, req)
		}
	}}})
	r.Env = map[string]string{"HTTP_PROXY": "http://egress-preview:4750", "HTTPS_PROXY": "http://egress:4750", "NO_PROXY": "localhost,::1,nightshift,lgtm"}
	q.next.Job.Inputs = json.RawMessage(`{"mode":"acceptance"}`)
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if seen == nil {
		t.Fatal("the acceptance job got no browser")
	}
	args, _ := json.Marshal(cfg["browser"])
	for _, want := range []string{"--proxy-server=http=http://egress-preview:4750;https=http://egress:4750", "--proxy-bypass-list=localhost;127.0.0.1;[::1]\"", "--disable-background-networking"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("browser config lacks %s: %s", want, args)
		}
	}
	if strings.Contains(string(args), "nightshift") || strings.Contains(string(args), "lgtm") {
		t.Errorf("the browser bypasses the proxies for the factory's services: %s", args)
	}
	if cfg["outputDir"] != seen.EvidenceDir {
		t.Errorf("output dir %v, want %s", cfg["outputDir"], seen.EvidenceDir)
	}
	if len(q.completed) != 1 {
		t.Fatalf("complete: %+v", q.completed)
	}
	found := false
	for _, a := range q.completed[0].Artifacts {
		found = found || strings.HasSuffix(a.Path, "evidence/01.png")
	}
	if !found {
		t.Errorf("the screenshot is not an artifact: %+v", q.completed[0].Artifacts)
	}
}

// A review job gets no browser.
func TestAReviewJobGetsNoBrowser(t *testing.T) {
	t.Parallel()
	var seen *harness.Browser
	r2, q2 := fixture(t, browsingAgent{agent{out: `{}`, peek: func(req harness.RunRequest) { seen = req.Browser }}})
	q2.next.Job.Inputs = json.RawMessage(`{"mode":"review"}`)
	if _, err := r2.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if seen != nil {
		t.Error("a review job got a browser")
	}
}

// browseLikeAnAgent checks the evidence directory and the config the job got, saves a screenshot
// as the browser tool would, and returns the config.
func browseLikeAnAgent(t *testing.T, req harness.RunRequest) map[string]any {
	t.Helper()
	b := req.Browser
	if info, err := os.Stat(b.EvidenceDir); err != nil || info.Mode().Perm() != 0o700 || strings.HasPrefix(b.EvidenceDir, req.Workspace) {
		t.Errorf("evidence directory %s: %v %v", b.EvidenceDir, info, err)
	}
	var cfg map[string]any
	raw, err := os.ReadFile(b.Config)
	if err == nil {
		err = json.Unmarshal(raw, &cfg)
	}
	if err != nil {
		t.Error(err)
	}
	if err := os.WriteFile(filepath.Join(b.EvidenceDir, "01-start.png"), []byte("\x89PNG\r\n\x1a\nimage"), 0o600); err != nil {
		t.Error(err)
	}
	return cfg
}

// An acceptance job on a harness without a browser tool gets no browser and keeps the
// screenshots it saved in the checkout (review, 2026-11-28).
func TestAnAcceptanceJobWithoutABrowserToolKeepsItsCheckoutScreenshots(t *testing.T) {
	t.Parallel()
	var seen *harness.Browser
	r, q := fixture(t, agent{out: `{}`, peek: func(req harness.RunRequest) {
		seen = req.Browser
		dir := filepath.Join(req.Workspace, "shots")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "01.png"), []byte("\x89PNG\r\n\x1a\nimage"), 0o600); err != nil {
			t.Error(err)
		}
	}})
	q.next.Job.Inputs = json.RawMessage(`{"mode":"acceptance"}`)
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if seen != nil {
		t.Error("a harness without a browser tool got a browser")
	}
	if len(q.completed) != 1 || !slices.ContainsFunc(q.completed[0].Artifacts, func(a jobsapi.Artifact) bool { return strings.HasSuffix(a.Path, "evidence/01.png") }) {
		t.Errorf("the checkout screenshot is not an artifact: %+v", q.completed)
	}
}

// browsingAgent is a harness that wires the browser tool.
type browsingAgent struct{ agent }

func (browsingAgent) Browses() bool { return true }
