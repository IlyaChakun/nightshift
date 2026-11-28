package runner

import (
	"slices"
	"testing"
)

// Project commands get the HTTPS proxy but not the preview proxy, which would send a test's
// plain http to the worker itself into egress-preview; credentials stay out (review, 2026-11-28).
func TestToolEnvCarriesTheEgressProxyOnly(t *testing.T) {
	r := &Runner{Env: map[string]string{
		"HTTPS_PROXY": "http://egress:4750", "HTTP_PROXY": "http://egress-preview:4750",
		"http_proxy": "http://egress-preview:4750", "ANTHROPIC_API_KEY": "sk-test",
	}}
	env := r.toolEnv(t.TempDir())
	if !slices.Contains(env, "HTTPS_PROXY=http://egress:4750") {
		t.Errorf("toolEnv lacks the egress proxy: %v", env)
	}
	for _, bad := range []string{"HTTP_PROXY=http://egress-preview:4750", "http_proxy=http://egress-preview:4750", "ANTHROPIC_API_KEY=sk-test"} {
		if slices.Contains(env, bad) {
			t.Errorf("toolEnv has %s: %v", bad, env)
		}
	}
}
