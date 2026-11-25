package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
)

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// The whole flow against a twin of GitHub: the page carries the manifest, a visit with a
// wrong state does nothing, and the right one stores the app without showing the key.
func TestRegisterApp(t *testing.T) {
	t.Parallel()
	twin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/app-manifests/good-code/conversions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42,"slug":"example-app","html_url":"https://github.test/apps/example-app","pem":"test-only-key"}`))
	}))
	t.Cleanup(twin.Close)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "github_app_key"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	manifest, err := github.Manifest("example-app", "https://example.test", "http://127.0.0.1/created")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"workflows", "administration", `"active":true`} {
		if strings.Contains(manifest, bad) {
			t.Errorf("the manifest asks for %s: %s", bad, manifest)
		}
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	s := &appSetup{manifest: manifest, state: "right-state", api: &github.Client{BaseURL: twin.URL}, out: out}
	done := make(chan error, 1)
	go func() {
		_, err := register(t.Context(), ln, s)
		done <- err
	}()

	if _, page := get(t, base+"/"); !strings.Contains(page, "settings/apps/new?state=right-state") || !strings.Contains(page, "pull_requests") {
		t.Errorf("start page: %s", page)
	}
	if code, _ := get(t, base+"/created?code=good-code&state=wrong"); code != http.StatusForbidden {
		t.Errorf("a wrong state got %d", code)
	}
	code, page := get(t, base+"/created?code=good-code&state=right-state")
	if code != http.StatusOK || strings.Contains(page, "test-only-key") {
		t.Errorf("created: %d %s", code, page)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"github_app_id": "42", "github_app_key": "test-only-key"} {
		path := filepath.Join(out, name)
		got, err := os.ReadFile(path) //nolint:gosec // a path inside the test temp dir
		info, serr := os.Stat(path)
		if err != nil || serr != nil || string(got) != want || info.Mode().Perm() != 0o400 {
			t.Errorf("%s: %q %v %v", name, got, err, serr)
		}
	}
	if err := writable(out, false); err == nil {
		t.Error("a stored app must not be overwritten without --replace")
	}
}

func TestConvertManifestRefusesAUsedCode(t *testing.T) {
	t.Parallel()
	twin := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(twin.Close)
	c := &github.Client{BaseURL: twin.URL}
	for _, code := range []string{"used", "../x", ""} {
		if _, err := c.ConvertManifest(t.Context(), code); err == nil {
			t.Errorf("%q: want an error", code)
		}
	}
}
