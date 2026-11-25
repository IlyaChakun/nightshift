package onboard

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/config"
)

// Problem is one finding of ValidateProject: an error blocks, a warning informs.
type Problem struct {
	File    string
	Warning bool
	Message string
}

func (p Problem) String() string {
	level := "error"
	if p.Warning {
		level = "warning"
	}
	return fmt.Sprintf("%s: %s: %s", level, p.File, p.Message)
}

// HasErrors reports whether any problem is an error.
func HasErrors(ps []Problem) bool {
	return slices.ContainsFunc(ps, func(p Problem) bool { return !p.Warning })
}

// projectRefs are the fields of project.yaml whose values point at other files or packs.
type projectRefs struct {
	Name         string   `yaml:"name"`
	Stack        []string `yaml:"stack"`
	Instructions []string `yaml:"instructions"`
	Skills       []string `yaml:"skills"`
	BlastRadius  string   `yaml:"blast_radius"`
}

// ValidateProject checks the project's own files in a checkout against their schemas and
// the references a schema cannot see: the instruction and skill paths, the blast radius
// file, and the stacks (packs lists the names of the known stack packs, the project's own
// included). It reads files only: no secrets, no network, no tools. tenant, when not empty,
// is the name the project file must carry.
func ValidateProject(repo fs.FS, v *config.Validator, packs []string, tenant string) []Problem {
	const file = ".nightshift/project.yaml"
	doc, err := fs.ReadFile(repo, file)
	if err != nil {
		return []Problem{{File: file, Message: "the file is missing"}}
	}
	if err := v.ValidateYAML(config.KindProject, doc); err != nil {
		return []Problem{{File: file, Message: flatten(err)}}
	}
	var p projectRefs
	if err := yaml.Unmarshal(doc, &p); err != nil {
		return []Problem{{File: file, Message: flatten(err)}}
	}
	var out []Problem
	if tenant != "" && p.Name != tenant {
		out = append(out, Problem{File: file, Message: fmt.Sprintf("name is %q, the tenant is %q", p.Name, tenant)})
	}
	for _, s := range p.Stack {
		if !slices.Contains(packs, s) {
			out = append(out, Problem{File: file, Warning: true, Message: fmt.Sprintf("no stack pack %q: the checks of that stack run on `verify` alone", s)})
		}
	}
	for field, paths := range map[string][]string{"instructions": p.Instructions, "skills": p.Skills} {
		for _, rel := range paths {
			if !pathExists(repo, rel) {
				out = append(out, Problem{File: file, Message: fmt.Sprintf("%s: %q does not exist in the repository", field, rel)})
			}
		}
	}
	out = append(out, blastRadius(repo, v, p.BlastRadius)...)
	slices.SortFunc(out, func(a, b Problem) int { return strings.Compare(a.String(), b.String()) })
	return out
}

func blastRadius(repo fs.FS, v *config.Validator, rel string) []Problem {
	if !fs.ValidPath(path.Clean(rel)) {
		return []Problem{{File: ".nightshift/project.yaml", Message: fmt.Sprintf("blast_radius: %q leaves the repository", rel)}}
	}
	doc, err := fs.ReadFile(repo, path.Clean(rel))
	if err != nil {
		return []Problem{{File: ".nightshift/project.yaml", Message: fmt.Sprintf("blast_radius: %q cannot be read", rel)}}
	}
	if err := v.ValidateYAML(config.KindBlastRadius, doc); err != nil {
		return []Problem{{File: rel, Message: flatten(err)}}
	}
	return nil
}

// pathExists accepts a file or a directory; a trailing slash is allowed.
func pathExists(repo fs.FS, rel string) bool {
	rel = path.Clean(rel)
	if !fs.ValidPath(rel) {
		return false
	}
	_, err := fs.Stat(repo, rel)
	return !errors.Is(err, fs.ErrNotExist) && err == nil
}

func flatten(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
