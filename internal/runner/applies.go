package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// skillMeta is the part of a skill's front matter that selects it.
type skillMeta struct {
	AppliesTo struct {
		Deps []string `yaml:"deps"`
	} `yaml:"applies_to"`
	Roles []string `yaml:"roles"`
}

// dependencySkills returns the ids of own skills that name a dependency of the project in
// `applies_to: { deps: [...] }` and apply to the role. The list is sorted, so the prompt is
// stable. Only package.json is read for now; other manifests are added with the first skill
// that needs them.
func (r *Runner) dependencySkills(role, workspace string) []string {
	deps := packageDeps(workspace)
	if len(deps) == 0 || r.SkillsDir == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(r.SkillsDir, "own"))
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || !safeSegment(e.Name()) {
			continue
		}
		m, ok := readSkillMeta(filepath.Join(r.SkillsDir, "own", e.Name(), "SKILL.md"))
		if !ok || len(m.Roles) > 0 && !slices.Contains(m.Roles, role) {
			continue
		}
		if slices.ContainsFunc(m.AppliesTo.Deps, func(d string) bool { return deps[d] }) {
			ids = append(ids, "own/"+e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}

func readSkillMeta(path string) (skillMeta, bool) {
	var m skillMeta
	text, err := os.ReadFile(path) //nolint:gosec // G304: a SKILL.md under the skills directory of the image
	if err != nil {
		return m, false
	}
	head, ok := frontMatter(string(text))
	if !ok || yaml.Unmarshal([]byte(head), &m) != nil {
		return m, false
	}
	return m, true
}

// frontMatter returns the text between the first two `---` lines.
func frontMatter(s string) (string, bool) {
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return "", false
	}
	head, _, ok := strings.Cut(rest, "\n---")
	return head, ok
}

// packageDeps lists the dependency names from package.json of the workspace.
func packageDeps(workspace string) map[string]bool {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }()
	doc, err := readInRoot(root, "package.json")
	if err != nil {
		return nil
	}
	var p struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal([]byte(doc), &p) != nil {
		return nil
	}
	out := map[string]bool{}
	for k := range p.Dependencies {
		out[k] = true
	}
	for k := range p.DevDependencies {
		out[k] = true
	}
	return out
}
