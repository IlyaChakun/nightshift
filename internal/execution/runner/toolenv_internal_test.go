package runner

import (
	"slices"
	"testing"
)

// Project commands get the preview proxy too: a check that opens the preview needs it
// (V2 review, 2026-11-28). Credentials still stay out.
func TestToolEnvCarriesTheProxiesAndNoCredentials(t *testing.T) {
	r := &Runner{Env: map[string]string{
		"HTTPS_PROXY": "http://egress:4750", "HTTP_PROXY": "http://egress-preview:4750",
		"http_proxy": "http://egress-preview:4750", "no_proxy": "localhost", "ANTHROPIC_API_KEY": "sk-test",
	}}
	env := r.toolEnv(t.TempDir())
	for _, want := range []string{"HTTPS_PROXY=http://egress:4750", "HTTP_PROXY=http://egress-preview:4750", "http_proxy=http://egress-preview:4750", "no_proxy=localhost"} {
		if !slices.Contains(env, want) {
			t.Errorf("toolEnv lacks %s: %v", want, env)
		}
	}
	if slices.Contains(env, "ANTHROPIC_API_KEY=sk-test") {
		t.Errorf("a credential reached the project commands: %v", env)
	}
}
