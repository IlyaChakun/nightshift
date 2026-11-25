package main

import "testing"

// TestPassEnv checks that the agent gets the proxy of the worker and nothing outside the list.
func TestPassEnv(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://egress:4750")
	t.Setenv("GITHUB_TOKEN", "test-only")
	env := map[string]string{}
	passEnv(env)
	if env["HTTPS_PROXY"] != "http://egress:4750" {
		t.Errorf("proxy not passed: %v", env)
	}
	if _, ok := env["GITHUB_TOKEN"]; ok {
		t.Error("a variable outside the list reached the agent")
	}
}
