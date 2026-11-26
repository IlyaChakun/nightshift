package composelocal

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The lint is an allowlist: a key that is not named here is refused, so a setting that a later
// version of the engine adds is closed until someone has read what it does. Keys that carry a
// rule of their own (for example privileged) are listed too, so the rule reports them by name.
var (
	topKeys = setOf("name", "services", "volumes", "networks", "secrets", "configs")

	serviceKeys = setOf(
		"image", "build", "command", "entrypoint", "environment", "ports", "expose", "depends_on",
		"healthcheck", "working_dir", "user", "restart", "labels", "networks", "tmpfs", "read_only",
		"init", "stop_grace_period", "stop_signal", "hostname", "tty", "stdin_open", "platform",
		"pull_policy", "profiles", "ulimits", "shm_size", "cap_drop", "secrets", "configs",
		"cpus", "mem_limit", "mem_reservation", "memswap_limit", "pids_limit",
		// each of these has a lint rule of its own
		"env_file", "volumes", "volumes_from", "extra_hosts", "security_opt", "privileged",
		"network_mode", "pid", "ipc", "uts", "userns_mode", "cgroup", "cgroup_parent", "runtime",
		"gpus", "cap_add", "devices", "device_cgroup_rules",
	)

	buildKeys = setOf(
		"context", "dockerfile", "dockerfile_inline", "args", "target", "labels", "no_cache", "pull", "platforms",
		"additional_contexts", "ssh", "network", "entitlements", "secrets", // linted
	)

	volumeKeys  = setOf("name", "driver", "driver_opts", "external", "labels")
	networkKeys = setOf("name", "driver", "external", "internal", "labels")
	fileRefKeys = setOf("file", "name", "environment", "content")
)

func setOf(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// lintKeys returns a violation for every key outside the allowlists. Keys that start with "x-"
// are the project's own annotations and mean nothing to the engine.
func lintKeys(rendered []byte) ([]Violation, error) {
	var doc map[string]any
	if err := json.Unmarshal(rendered, &doc); err != nil {
		return nil, fmt.Errorf("composelocal: the rendered recipe is not JSON: %w", err)
	}
	var out []Violation
	unknown := func(where string, obj any, allowed map[string]bool) {
		m, _ := obj.(map[string]any)
		for _, k := range sortedKeys(m) {
			if !allowed[k] && !strings.HasPrefix(k, "x-") {
				out = append(out, Violation{Service: where, Rule: "recipe key not allowed", Detail: k})
			}
		}
	}
	unknown("(top level)", doc, topKeys)
	for _, section := range []struct {
		key     string
		allowed map[string]bool
	}{{"volumes", volumeKeys}, {"networks", networkKeys}, {"secrets", fileRefKeys}, {"configs", fileRefKeys}} {
		items, _ := doc[section.key].(map[string]any)
		for _, n := range sortedKeys(items) {
			unknown("(top level) "+section.key+" "+n, items[n], section.allowed)
		}
	}
	services, _ := doc["services"].(map[string]any)
	for _, n := range sortedKeys(services) {
		unknown(n, services[n], serviceKeys)
		if svc, ok := services[n].(map[string]any); ok {
			unknown(n+" build", svc["build"], buildKeys)
		}
	}
	return out, nil
}
