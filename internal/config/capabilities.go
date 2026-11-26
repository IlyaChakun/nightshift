package config

import (
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Capability is one of `preview`, `release`, `rollback` and `smoke` of project.yaml
// (design section 17, "Deploy targets"). A bare string is a project command (Kind "script").
type Capability struct {
	Kind    string   // compose-local, script or external
	File    string   // compose-local: the recipe, a path in the repository
	Entry   string   // compose-local preview: the service that serves it
	CPUs    float64  // compose-local preview limits; 0 means the default
	Memory  string   //
	Command string   // script: the command; also the project's own smoke check
	Up      string   // script preview
	Down    string   // script preview
	Envs    []string // external preview
}

// Capabilities is the `capabilities` block. A nil capability is absent: the tenant does not
// have it (`release: null`).
type Capabilities struct {
	MergeTrain bool        `yaml:"merge_train"`
	Preview    *Capability `yaml:"preview"`
	Release    *Capability `yaml:"release"`
	Rollback   *Capability `yaml:"rollback"`
	Smoke      *Capability `yaml:"smoke"`
}

// UnmarshalYAML reads a command string or an object.
func (c *Capability) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var cmd string
		if err := n.Decode(&cmd); err != nil {
			return err
		}
		*c = Capability{Kind: "script", Command: cmd}
		return nil
	}
	var raw struct {
		Kind   string `yaml:"kind"`
		File   string `yaml:"file"`
		Entry  string `yaml:"entry"`
		Limits struct {
			CPUs   float64 `yaml:"cpus"`
			Memory string  `yaml:"memory"`
		} `yaml:"limits"`
		Up   string   `yaml:"up"`
		Down string   `yaml:"down"`
		Envs []string `yaml:"envs"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	*c = Capability{Kind: raw.Kind, File: raw.File, Entry: raw.Entry, CPUs: raw.Limits.CPUs, Memory: raw.Limits.Memory, Up: raw.Up, Down: raw.Down, Envs: raw.Envs}
	return nil
}

// ErrNoCapabilities means the project file has no `capabilities` block: the project has no deploy targets.
var ErrNoCapabilities = errors.New("config: project.yaml has no capabilities")

// ParseCapabilities reads the `capabilities` block of a project.yaml that already passed the
// schema. A document without the block returns ErrNoCapabilities.
func ParseCapabilities(project []byte) (Capabilities, error) {
	var doc struct {
		Capabilities *Capabilities `yaml:"capabilities"`
	}
	if err := yaml.Unmarshal(project, &doc); err != nil {
		return Capabilities{}, fmt.Errorf("config: project capabilities: %w", err)
	}
	if doc.Capabilities == nil {
		return Capabilities{}, ErrNoCapabilities
	}
	return *doc.Capabilities, nil
}
