// Package standards turns the standards packs into the short L0 checklist that goes into a
// role's prompt (design section 9, layer L0).
package standards

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"go.yaml.in/yaml/v3"
)

type entry struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Status string `yaml:"status"`
}

// Checklist returns one line for each active standard of the stacks, in path order. A
// standard is `<set>/<stack>/<name>.yaml`; the line names the rule and, when a page with the
// same name sits beside the file, points to it so the agent can read the details.
func Checklist(fsys fs.FS, stacks []string) ([]string, error) {
	var lines []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".yaml" {
			return err
		}
		if !slices.Contains(stacks, path.Base(path.Dir(p))) {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var e entry
		if err := yaml.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if e.Status != "active" || e.Title == "" {
			return nil
		}
		line := e.Title + " (" + e.ID + ")"
		if page := p[:len(p)-len(".yaml")] + ".md"; exists(fsys, page) {
			line += ", details: standards/" + page
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("standards: %w", err)
	}
	return lines, nil
}

func exists(fsys fs.FS, name string) bool {
	_, err := fs.Stat(fsys, name)
	return err == nil
}
