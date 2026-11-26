// Package composelocal is the compose-local deploy adapter (design section 17, ADR 0014): one
// Docker host, for example the owner's laptop. Access to the engine is root-equivalent on its
// host, so nothing reaches `docker compose up` before Lint has read the rendered recipe.
package composelocal

import (
	"encoding/json"
	"fmt"
	"os"
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
// extension and variable already resolved, and returns what it may not do. It is a denylist
// over a recipe that the code under test may carry (the recipe is read from main, but the
// checkout it points into is the change's), so every path is resolved through symlinks first.
// A recipe that is not JSON of this shape is refused: the lint never passes what it cannot read.
func Lint(rendered []byte, checkout string) ([]Violation, error) {
	var cfg struct {
		Services map[string]service `json:"services"`
		Volumes  map[string]struct {
			Driver     string            `json:"driver"`
			DriverOpts map[string]string `json:"driver_opts"`
			External   any               `json:"external"`
		} `json:"volumes"`
		Networks map[string]struct {
			Driver   string `json:"driver"`
			Name     string `json:"name"`
			External any    `json:"external"`
		} `json:"networks"`
		Secrets map[string]fileRef `json:"secrets"`
		Configs map[string]fileRef `json:"configs"`
	}
	if err := json.Unmarshal(rendered, &cfg); err != nil {
		return nil, fmt.Errorf("composelocal: the rendered recipe is not JSON: %w", err)
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("composelocal: the rendered recipe has no services")
	}
	root := resolve(filepath.Clean(checkout))
	var out []Violation
	top := func(rule, detail string) {
		out = append(out, Violation{Service: "(top level)", Rule: rule, Detail: detail})
	}
	for _, n := range sortedKeys(cfg.Volumes) {
		v := cfg.Volumes[n]
		if truthy(v.External) {
			top("external volume", n)
		}
		if v.Driver != "" && v.Driver != "local" {
			top("volume driver", n+": "+v.Driver)
		}
		for k, val := range v.DriverOpts {
			if k == "device" || (k == "o" && strings.Contains(val, "bind")) {
				top("volume backed by a host path", n+": "+k+"="+val)
			}
		}
	}
	for _, n := range sortedKeys(cfg.Networks) {
		v := cfg.Networks[n]
		if truthy(v.External) {
			top("external network", n)
		}
		if v.Name == "host" || v.Name == "none" || (v.Driver != "" && v.Driver != "bridge") {
			top("network", fmt.Sprintf("%s: name %q driver %q", n, v.Name, v.Driver))
		}
	}
	for _, kind := range []struct {
		name string
		m    map[string]fileRef
	}{{"secret", cfg.Secrets}, {"config", cfg.Configs}} {
		for _, n := range sortedKeys(kind.m) {
			if f := kind.m[n].File; f != "" && !inside(root, f) {
				top(kind.name+" file outside the checkout", f)
			}
		}
	}
	for _, n := range sortedKeys(cfg.Services) {
		out = append(out, cfg.Services[n].lint(n, root)...)
	}
	return out, nil
}

type fileRef struct {
	File string `json:"file"`
}

type service struct {
	Privileged        bool      `json:"privileged"`
	NetworkMode       string    `json:"network_mode"`
	Pid               string    `json:"pid"`
	Ipc               string    `json:"ipc"`
	Uts               string    `json:"uts"`
	UsernsMode        string    `json:"userns_mode"`
	Cgroup            string    `json:"cgroup"`
	CgroupParent      string    `json:"cgroup_parent"`
	Runtime           string    `json:"runtime"`
	Gpus              any       `json:"gpus"`
	CapAdd            []string  `json:"cap_add"`
	Devices           []any     `json:"devices"`
	DeviceCgroupRules []string  `json:"device_cgroup_rules"`
	SecurityOpt       []string  `json:"security_opt"`
	VolumesFrom       []string  `json:"volumes_from"`
	ExtraHosts        any       `json:"extra_hosts"`
	EnvFile           []envFile `json:"env_file"`
	Volumes           []volume  `json:"volumes"`
	Build             *build    `json:"build"`
}

type envFile struct {
	Path string `json:"path"`
}

type build struct {
	Context            string         `json:"context"`
	AdditionalContexts map[string]any `json:"additional_contexts"`
	SSH                any            `json:"ssh"`
	Network            string         `json:"network"`
	Entitlements       []string       `json:"entitlements"`
	Secrets            []any          `json:"secrets"`
}

type volume struct {
	Type   string `json:"type"`
	Source string `json:"source"`
}

func (s service) lint(name, root string) []Violation {
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
	for field, v := range map[string]string{"cgroup_parent": s.CgroupParent, "runtime": s.Runtime} {
		if v != "" {
			add("engine setting", field+": "+v)
		}
	}
	if s.Gpus != nil {
		add("engine setting", "gpus")
	}
	if len(s.CapAdd) > 0 {
		add("added capabilities", strings.Join(s.CapAdd, ", "))
	}
	if len(s.Devices) > 0 {
		add("devices", fmt.Sprintf("%d device mapping(s)", len(s.Devices)))
	}
	if len(s.DeviceCgroupRules) > 0 {
		add("device cgroup rules", strings.Join(s.DeviceCgroupRules, ", "))
	}
	for _, o := range s.SecurityOpt {
		if !strings.HasPrefix(o, "no-new-privileges") { // anything else loosens or replaces the default confinement
			add("security option", o)
		}
	}
	for _, v := range s.VolumesFrom {
		if strings.HasPrefix(v, "container:") {
			add("volumes from a container outside the project", v)
		}
	}
	if s.ExtraHosts != nil && strings.Contains(fmt.Sprint(s.ExtraHosts), "host-gateway") {
		add("host gateway", "extra_hosts reaches the host")
	}
	for _, e := range s.EnvFile {
		if !inside(root, e.Path) {
			add("env file outside the checkout", e.Path)
		}
	}
	for _, v := range s.Volumes {
		switch v.Type {
		case "bind":
			if isDockerSocket(v.Source) || isDockerSocket(resolve(v.Source)) {
				add("docker socket", v.Source)
			} else if !inside(root, v.Source) {
				add("bind mount outside the checkout", v.Source)
			}
		case "volume", "tmpfs", "":
		default:
			add("mount type", v.Type)
		}
	}
	if b := s.Build; b != nil {
		if b.Context != "" && !strings.Contains(b.Context, "://") && !inside(root, b.Context) {
			add("build context outside the checkout", b.Context)
		}
		for n, c := range b.AdditionalContexts {
			if p, ok := c.(string); ok && !strings.Contains(p, "://") && !strings.HasPrefix(p, "service:") && !strings.HasPrefix(p, "docker-image:") && !inside(root, p) {
				add("build context outside the checkout", n+": "+p)
			}
		}
		if b.SSH != nil || b.Network == "host" || len(b.Entitlements) > 0 || len(b.Secrets) > 0 {
			add("build option", "ssh, host network, entitlements or build secrets")
		}
	}
	return out
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case map[string]any:
		return true
	case string:
		return x != "" && x != "false"
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isDockerSocket(p string) bool {
	base := filepath.Base(p)
	return base == "docker.sock" || base == "podman.sock" || base == "containerd.sock"
}

// resolve follows symlinks as far as the path exists and appends the rest, so a link the code
// under test committed cannot make a path outside the checkout look like one inside it. A bind
// mount of a missing path is created by the engine, so the part that does not exist is kept.
func resolve(p string) string { return resolveDepth(p, 0) }

func resolveDepth(p string, depth int) string {
	p = filepath.Clean(p)
	if depth > 8 {
		return p
	}
	rest := ""
	for cur := p; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		// A link whose target does not exist here may exist on the engine's host (a socket), so
		// follow it by hand.
		if fi, err := os.Lstat(cur); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(cur); err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(cur), target)
				}
				return resolveDepth(filepath.Join(target, rest), depth+1)
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// inside reports whether p, with links resolved, is root or below it; ".." cannot climb out.
// root is already resolved.
func inside(root, p string) bool {
	if !filepath.IsAbs(p) {
		return false // a rendered recipe has absolute sources; anything else is not trusted
	}
	rel, err := filepath.Rel(root, resolve(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
