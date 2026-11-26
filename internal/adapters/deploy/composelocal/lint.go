// Package composelocal is the compose-local deploy adapter (design section 17, ADR 0014): one
// Docker host, for example the owner's laptop. Access to the engine is root-equivalent on its
// host, so nothing reaches `docker compose up` before Lint has read the rendered recipe.
package composelocal

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Violation is one setting the recipe may not use.
type Violation struct {
	Service string
	Rule    string
	Detail  string
}

func (v Violation) String() string {
	return fmt.Sprintf("service %q: %s (%s)", v.Service, v.Rule, v.Detail)
}

// Lint reads a recipe as `docker compose config --format json` renders it, with every include,
// extension and variable already resolved, and returns what it may not do: privileged mode, the
// host's network, PID or IPC namespaces, added capabilities, devices, security options that turn
// confinement off, bind mounts outside checkout and the Docker socket. A recipe that is not JSON
// of this shape is refused: the lint never passes what it cannot read.
func Lint(rendered []byte, checkout string) ([]Violation, error) {
	var cfg struct {
		Services map[string]service `json:"services"`
	}
	if err := json.Unmarshal(rendered, &cfg); err != nil {
		return nil, fmt.Errorf("composelocal: the rendered recipe is not JSON: %w", err)
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("composelocal: the rendered recipe has no services")
	}
	checkout = filepath.Clean(checkout)
	var out []Violation
	names := make([]string, 0, len(cfg.Services))
	for n := range cfg.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, cfg.Services[n].lint(n, checkout)...)
	}
	return out, nil
}

type service struct {
	Privileged  bool     `json:"privileged"`
	NetworkMode string   `json:"network_mode"`
	Pid         string   `json:"pid"`
	Ipc         string   `json:"ipc"`
	Uts         string   `json:"uts"`
	UsernsMode  string   `json:"userns_mode"`
	Cgroup      string   `json:"cgroup"`
	CapAdd      []string `json:"cap_add"`
	Devices     []any    `json:"devices"`
	SecurityOpt []string `json:"security_opt"`
	Volumes     []volume `json:"volumes"`
}

type volume struct {
	Type   string `json:"type"`
	Source string `json:"source"`
}

func (s service) lint(name, checkout string) []Violation {
	var out []Violation
	add := func(rule, detail string) { out = append(out, Violation{Service: name, Rule: rule, Detail: detail}) }
	if s.Privileged {
		add("privileged mode", "privileged: true")
	}
	for _, ns := range [][2]string{{"network_mode", s.NetworkMode}, {"pid", s.Pid}, {"ipc", s.Ipc}, {"uts", s.Uts}, {"userns_mode", s.UsernsMode}, {"cgroup", s.Cgroup}} {
		if ns[1] == "host" || strings.HasPrefix(ns[1], "container:") {
			add("host namespace", ns[0]+": "+ns[1])
		}
	}
	if len(s.CapAdd) > 0 {
		add("added capabilities", strings.Join(s.CapAdd, ", "))
	}
	if len(s.Devices) > 0 {
		add("devices", fmt.Sprintf("%d device mapping(s)", len(s.Devices)))
	}
	for _, o := range s.SecurityOpt {
		if strings.Contains(o, "unconfined") || strings.HasPrefix(o, "systempaths=") || strings.HasPrefix(o, "label=disable") {
			add("security option", o)
		}
	}
	for _, v := range s.Volumes {
		switch v.Type {
		case "bind":
			if isDockerSocket(v.Source) {
				add("docker socket", v.Source)
			} else if !inside(checkout, v.Source) {
				add("bind mount outside the checkout", v.Source)
			}
		case "volume", "tmpfs", "":
		default:
			add("mount type", v.Type)
		}
	}
	return out
}

func isDockerSocket(p string) bool {
	base := filepath.Base(p)
	return base == "docker.sock" || base == "podman.sock" || base == "containerd.sock"
}

// inside reports whether p is the checkout or below it, after cleaning; ".." cannot climb out.
func inside(checkout, p string) bool {
	if !filepath.IsAbs(p) {
		return false // a rendered recipe has absolute sources; anything else is not trusted
	}
	rel, err := filepath.Rel(checkout, filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
