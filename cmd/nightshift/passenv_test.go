package main

import (
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestPassEnv checks that the agent gets the proxy of the worker and nothing outside the list.
func TestPassEnv(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://egress:4750")
	t.Setenv("http_proxy", "http://egress-preview:4750")
	t.Setenv("GITHUB_TOKEN", "test-only")
	env := map[string]string{}
	passEnv(env)
	if env["HTTPS_PROXY"] != "http://egress:4750" || env["http_proxy"] != "http://egress-preview:4750" {
		t.Errorf("proxy not passed: %v", env)
	}
	if _, ok := env["GITHUB_TOKEN"]; ok {
		t.Error("a variable outside the list reached the agent")
	}
}

// Every proxy setting the worker gets in compose reaches the agent: without HTTP_PROXY the
// Verifier cannot open a preview, which only egress-preview serves (V2 review, 2026-11-28).
func TestEveryWorkerProxySettingReachesTheAgent(t *testing.T) {
	t.Parallel()
	var compose struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, "../../deploy/docker-compose.yml")), &compose); err != nil {
		t.Fatal(err)
	}
	worker, ok := compose.Services["worker"]
	if !ok {
		t.Fatal("compose has no worker service")
	}
	n := 0
	for k := range worker.Environment {
		if strings.Contains(strings.ToLower(k), "proxy") {
			n++
			if !slices.Contains(agentEnv, k) {
				t.Errorf("the worker's %s does not reach the agent (agentEnv)", k)
			}
		}
	}
	if n == 0 {
		t.Error("the worker has no proxy settings: the check reads the wrong file")
	}
}
