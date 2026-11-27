// Package fake is a Harness that replays the agent steps of a scenario file,
// so factory tests run without models, network or tokens (design section 20).
package fake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
)

// PatchFile is the name under which a step's patch is written into the
// request workspace.
const PatchFile = "fake.patch"

// Errors returned by Run.
var (
	ErrNoMoreSteps = errors.New("fake: scenario has no more agent steps")
	ErrWrongRole   = errors.New("fake: next agent step is for another role")
)

// Scenario is the subset of a testdata/scenarios/*.yaml file that the fake
// harness needs. Steps other than agent steps are driven by twins (phase 3)
// and are ignored here.
type Scenario struct {
	Given map[string]any `yaml:"given"`
	Steps []Step         `yaml:"steps"`
}

// Step is one scenario step; only Agent is used by this package.
type Step struct {
	Agent *AgentStep `yaml:"agent"`
}

// AgentStep says what the "agent" returns on its turn.
type AgentStep struct {
	Role   string    `yaml:"role"`
	Result yaml.Node `yaml:"result"` // a path to a JSON file or an inline mapping
	Patch  string    `yaml:"patch"`  // optional path to a patch file
	Usage  struct {
		InputTokens  int `yaml:"input_tokens"`
		OutputTokens int `yaml:"output_tokens"`
	} `yaml:"usage"`
}

// Harness replays agent steps in order. It is safe for concurrent use, but
// steps are consumed strictly in file order.
type Harness struct {
	dir   string // scenario directory; file paths are relative to it
	mu    sync.Mutex
	steps []AgentStep
	next  int
}

// Load reads a scenario file.
func Load(path string) (*Harness, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // test scenarios come from the repository
	if err != nil {
		return nil, fmt.Errorf("fake: read scenario: %w", err)
	}
	var sc Scenario
	if err := yaml.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("fake: parse scenario %s: %w", path, err)
	}
	h := &Harness{dir: filepath.Dir(path)}
	for _, s := range sc.Steps {
		if s.Agent != nil {
			h.steps = append(h.steps, *s.Agent)
		}
	}
	return h, nil
}

// Remaining returns how many agent steps are left.
func (h *Harness) Remaining() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.steps) - h.next
}

// Run implements harness.Harness.
func (h *Harness) Run(ctx context.Context, req harness.RunRequest) (harness.RunResult, error) {
	if err := ctx.Err(); err != nil {
		return harness.RunResult{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.next >= len(h.steps) {
		return harness.RunResult{}, ErrNoMoreSteps
	}
	step := h.steps[h.next]
	if step.Role != req.Role.Name {
		return harness.RunResult{}, fmt.Errorf("%w: want %q, got %q", ErrWrongRole, step.Role, req.Role.Name)
	}
	out, err := h.result(step)
	if err != nil {
		return harness.RunResult{}, err
	}
	if step.Patch != "" && req.Workspace != "" {
		if err := h.copyPatch(step.Patch, req.Workspace); err != nil {
			return harness.RunResult{}, err
		}
	}
	h.next++
	return harness.RunResult{
		Output:     out,
		SessionID:  fmt.Sprintf("fake-%s-%d", step.Role, h.next),
		ExitReason: harness.ExitDone,
		Usage: harness.Usage{
			InputTokens:  step.Usage.InputTokens,
			OutputTokens: step.Usage.OutputTokens,
		},
	}, nil
}

func (h *Harness) result(step AgentStep) (json.RawMessage, error) {
	switch step.Result.Kind {
	case yaml.ScalarNode:
		raw, err := os.ReadFile(filepath.Join(h.dir, step.Result.Value))
		if err != nil {
			return nil, fmt.Errorf("fake: read result: %w", err)
		}
		return raw, nil
	case yaml.MappingNode:
		var v any
		if err := step.Result.Decode(&v); err != nil {
			return nil, fmt.Errorf("fake: decode inline result: %w", err)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("fake: encode inline result: %w", err)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("fake: step for %q has no result", step.Role)
	}
}

func (h *Harness) copyPatch(rel, workspace string) error {
	raw, err := os.ReadFile(filepath.Join(h.dir, rel)) //nolint:gosec // fixture path from the scenario
	if err != nil {
		return fmt.Errorf("fake: read patch: %w", err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return fmt.Errorf("fake: open workspace: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile(PatchFile, raw, 0o600); err != nil {
		return fmt.Errorf("fake: write patch: %w", err)
	}
	return nil
}
