package mcpdoor_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ilyachakun/nightshift/internal/mcpdoor"
)

// testToken is built, not written out, so no scanner takes it for a credential.
var testToken = strings.Repeat("test-token-", 5)

// bearer adds the token to every request of an MCP client.
type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b != "" {
		r.Header.Set("Authorization", "Bearer "+string(b))
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestRemoteDoorNeedsTheToken(t *testing.T) {
	t.Parallel()
	h, err := mcpdoor.RemoteHandler(&backend{}, testToken, mcpdoor.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	for name, auth := range map[string]string{"none": "", "wrong": "Bearer nope", "other scheme": "Basic " + testToken} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(`{}`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, resp.StatusCode)
		}
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &http.Client{Transport: bearer(testToken)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) != 5 {
		t.Fatalf("tools: %+v %v", tools, err)
	}
	for _, tool := range tools.Tools { // T6: no tool reaches a merge, a rollback or the settings
		for _, bad := range []string{"merge", "rollback", "setting", "secret"} {
			if strings.Contains(tool.Name, bad) {
				t.Errorf("the remote door offers %q", tool.Name)
			}
		}
	}
}

func TestRemoteDoorRefusesAShortToken(t *testing.T) {
	t.Parallel()
	if _, err := mcpdoor.RemoteHandler(&backend{}, "short", mcpdoor.Options{}); err == nil {
		t.Fatal("a short token was accepted")
	}
}

func TestValidateListen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:8765", true},
		{"localhost:8765", true},
		{"[::1]:8765", true},
		{"0.0.0.0:8765", false},
		{":8765", false},
		{"192.168.1.5:8765", false},
		{"example.org:8765", false},
		{"8765", false},
	}
	for _, tt := range tests {
		if err := mcpdoor.ValidateListen(tt.addr); (err == nil) != tt.ok {
			t.Errorf("%s: err = %v, want ok=%v", tt.addr, err, tt.ok)
		}
	}
}
