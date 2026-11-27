package deps

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, advisories string) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/systems/npm/packages/left-pad", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"versions":[{"versionKey":{"version":"1.0.0"},"publishedAt":"2016-03-01T00:00:00Z"},{"versionKey":{"version":"1.3.0"},"publishedAt":"2018-04-01T00:00:00Z","isDefault":true}]}`))
	})
	mux.HandleFunc("/v3/systems/npm/packages/left-pad/versions/1.3.0", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"licenses":["WTFPL"],"publishedAt":"2018-04-01T00:00:00Z","advisoryKeys":` + advisories + `}`))
	})
	mux.HandleFunc("/v3/systems/npm/packages/left-pad/versions/1.3.0:dependencies", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"nodes":[{"relation":"SELF"},{"relation":"DIRECT"}]}`))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return Client{Base: ts.URL, Now: func() time.Time { return time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC) }}
}

func TestCheckReportsLicenseAgeAdvisoriesAndGaps(t *testing.T) {
	r, err := server(t, `[{"id":"GHSA-xxxx"}]`).Check(t.Context(), "NPM", "left-pad", "", DefaultLicenses())
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.3.0" || r.LicenseAllowed || len(r.Advisories) != 1 || r.TreeSize != 1 {
		t.Fatalf("report: %+v", r)
	}
	text := r.Format()
	for _, want := range []string{"WTFPL", "not in the allowed list", "1 known advisories", "no release for more than two years", "not checked: install scripts"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestCheckPassesAnAllowedCleanPackage(t *testing.T) {
	r, err := server(t, `[]`).Check(t.Context(), "npm", "left-pad", "1.3.0", []string{"WTFPL"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.LicenseAllowed || len(r.Advisories) != 0 {
		t.Fatalf("report: %+v", r)
	}
}

func TestCheckFailsOnAnUnknownPackage(t *testing.T) {
	if _, err := server(t, `[]`).Check(t.Context(), "npm", "nope", "", nil); err == nil {
		t.Fatal("an unknown package must be an error")
	}
}
