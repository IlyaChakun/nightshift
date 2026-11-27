// Package composetest is a stateful double of the docker CLI for the compose-local adapter: it
// keeps each compose project's containers with the labels of the recipe that brought them up, so
// Status after Release, Rollback and Down reads what a real engine would. Contract tests of
// composelocal and of the deployer service run on it. It belongs to the testing plane.
package composetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Recipe is what `compose config` answers: one service, web, publishing a port.
const Recipe = `{"services":{"web":{"image":"example","ports":[{"target":80}]}}}`

type stack struct {
	labels  map[string]string
	healthy bool
}

// Engine implements composelocal.Exec.
type Engine struct {
	mu     sync.Mutex
	stacks map[string]stack
	bad    map[string]bool // revisions whose containers never become healthy
}

// New returns an engine with nothing running.
func New() *Engine { return &Engine{stacks: map[string]stack{}, bad: map[string]bool{}} }

// Bad makes rev a revision whose containers start and never become healthy, each time it is
// brought up, as a revision whose healthcheck fails does.
func (e *Engine) Bad(rev string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bad[rev] = true
}

// flag returns the value after name in args.
func flag(args []string, name string) string {
	if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// Run answers the calls the adapter makes.
func (e *Engine) Run(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// A real program starts in dir, so a missing one fails before docker runs.
	if dir != "" {
		if _, err := os.Stat(dir); err != nil {
			return nil, fmt.Errorf("composetest: chdir: %w", err)
		}
	}
	if name == "sh" {
		return nil, nil // the project's smoke check passes
	}
	if name != "docker" || len(args) == 0 {
		return nil, fmt.Errorf("composetest: unexpected call %s %v", name, args)
	}
	if args[0] == "ps" {
		proj := strings.TrimPrefix(flag(args, "--filter"), "label=com.docker.compose.project=")
		return e.ps(proj), nil
	}
	proj := flag(args, "-p")
	switch {
	case slices.Contains(args, "config"):
		return []byte(Recipe), nil
	case slices.Contains(args, "up"):
		return e.up(proj, flag(args, "-f"))
	case slices.Contains(args, "down"):
		delete(e.stacks, proj)
		return nil, nil
	case slices.Contains(args, "ps"):
		if _, ok := e.stacks[proj]; !ok {
			return nil, nil
		}
		return []byte(`{"State":"running","Publishers":[{"PublishedPort":49200}]}`), nil
	}
	return nil, fmt.Errorf("composetest: unexpected call docker %v", args)
}

func (e *Engine) up(proj, file string) ([]byte, error) {
	// The file is the adapter's rendered recipe; a path that climbs out of where it points is not one.
	if !filepath.IsAbs(file) || strings.Contains(file, "..") {
		return nil, fmt.Errorf("composetest: %q is not a rendered recipe", file)
	}
	raw, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return nil, fmt.Errorf("composetest: %w", err)
	}
	var cfg struct {
		Services map[string]struct {
			Labels map[string]string `json:"labels"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("composetest: %w", err)
	}
	web, ok := cfg.Services["web"]
	if !ok {
		return nil, errors.New("composetest: the rendered recipe has no web service")
	}
	healthy := !e.bad[web.Labels["ns.revision"]]
	e.stacks[proj] = stack{labels: web.Labels, healthy: healthy}
	if !healthy {
		return []byte("container web is unhealthy"), errors.New("exit status 1")
	}
	return nil, nil
}

func (e *Engine) ps(proj string) []byte {
	s, ok := e.stacks[proj]
	if !ok {
		return nil
	}
	var labels []string
	for k, v := range s.labels {
		labels = append(labels, k+"="+v)
	}
	slices.Sort(labels)
	status := "Up 3 seconds (healthy)"
	if !s.healthy {
		status = "Up 3 seconds (unhealthy)"
	}
	out, _ := json.Marshal(map[string]string{"State": "running", "Status": status, "Labels": strings.Join(labels, ",")}) //nolint:errchkjson // a map of strings
	return out
}
