package runner_test

import (
	"encoding/json"
	"testing"
)

// Only the Verifier's acceptance job, which opens the preview, gets the preview proxy; any other
// agent keeps plain http direct (review, 2026-11-28). The worker's own map is not changed.
func TestOnlyAnAcceptanceJobGetsThePreviewProxy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode string
		want bool
	}{{"acceptance", true}, {"review", false}, {"", false}} {
		var env map[string]string
		r, q := fixture(t, agent{out: `{}`, env: &env})
		r.Env = map[string]string{"HTTPS_PROXY": "http://egress:4750", "HTTP_PROXY": "http://egress-preview:4750", "http_proxy": "http://egress-preview:4750"}
		inputs, _ := json.Marshal(map[string]string{"mode": tc.mode})
		q.next.Job.Inputs = inputs
		if _, err := r.RunOne(t.Context()); err != nil {
			t.Fatal(err)
		}
		_, upper := env["HTTP_PROXY"]
		_, lower := env["http_proxy"]
		if upper != tc.want || lower != tc.want || env["HTTPS_PROXY"] == "" {
			t.Errorf("mode %q: env %v, want the preview proxy %v", tc.mode, env, tc.want)
		}
		if r.Env["HTTP_PROXY"] == "" {
			t.Fatal("the worker's own environment was modified")
		}
	}
}
