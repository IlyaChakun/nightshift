// Package stack loads the stack packs (stacks/<name>/pack.yaml): for each language what the
// worker needs and which static checks run on a patch (design section 5).
package stack

import (
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"slices"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/check"
	"github.com/ilyachakun/nightshift/internal/config"
)

// Tool is one static check of a pack.
type Tool struct {
	Name     string `yaml:"name"`
	Command  string `yaml:"command"`
	Requires string `yaml:"requires"`
	Category string `yaml:"category"`
}

// Pack is one stack.
type Pack struct {
	Name   string   `yaml:"name"`
	Detect []string `yaml:"detect"`
	// DetectUnless names files that mean another pack covers the project.
	DetectUnless []string `yaml:"detect_unless"`
	Toolchain    []string `yaml:"toolchain"`
	Tools        []Tool   `yaml:"tools"`
}

// Packs are the loaded packs by name.
type Packs map[string]Pack

// Load reads <name>/pack.yaml for every directory of fsys, validates it against
// schemas/stack-pack.json and checks that the file is named after its directory.
func Load(fsys fs.FS) (Packs, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("stack: %w", err)
	}
	v := config.NewValidator()
	packs := Packs{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		file := path.Join(e.Name(), "pack.yaml")
		doc, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("stack: %w", err)
		}
		if err := v.ValidateYAML(config.KindStackPack, doc); err != nil {
			return nil, fmt.Errorf("stack: %s: %w", file, err)
		}
		var p Pack
		if err := yaml.Unmarshal(doc, &p); err != nil {
			return nil, fmt.Errorf("stack: %s: %w", file, err)
		}
		if p.Name != e.Name() {
			return nil, fmt.Errorf("stack: %s names the pack %q, want %q", file, p.Name, e.Name())
		}
		packs[p.Name] = p
	}
	return packs, nil
}

// Detect returns the packs whose detect files are in the repository root, sorted by name.
func (ps Packs) Detect(repo fs.FS) []string {
	var found []string
	has := func(f string) bool { _, err := fs.Stat(repo, f); return err == nil }
	for name, p := range ps {
		if slices.ContainsFunc(p.DetectUnless, has) {
			continue
		}
		if slices.ContainsFunc(p.Detect, has) {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}

// Missing lists the toolchain executables of the stacks that look is unable to find.
func (ps Packs) Missing(stacks []string, look func(string) error) []string {
	var missing []string
	for _, s := range stacks {
		for _, bin := range ps[s].Toolchain {
			if look(bin) != nil && !slices.Contains(missing, bin) {
				missing = append(missing, bin)
			}
		}
	}
	return missing
}

// Tools returns the checks of the stacks for nightshift check. A tool whose executable the
// worker lacks is not returned but named in skipped, so the gap is reported rather than
// turned into a failure of every patch.
func (ps Packs) Tools(stacks []string) (tools []check.Tool, skipped []string) {
	return ps.ToolsWith(stacks, func(name string) bool { _, err := exec.LookPath(name); return err == nil })
}

// ToolsWith is Tools with the caller's way to find an executable: the runner looks in the mise
// shims first, because the project's toolchain is installed after the worker started.
func (ps Packs) ToolsWith(stacks []string, found func(name string) bool) (tools []check.Tool, skipped []string) {
	for _, s := range stacks {
		for _, t := range ps[s].Tools {
			if t.Requires != "" && !found(t.Requires) {
				skipped = append(skipped, t.Name)
				continue
			}
			tools = append(tools, check.Tool{Name: t.Name, Command: t.Command, Category: t.Category})
		}
	}
	return tools, skipped
}
