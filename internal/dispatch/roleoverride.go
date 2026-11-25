package dispatch

import (
	"bytes"
	"context"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Limits of a role override, so one file cannot flood a prompt.
const (
	maxOverrideItems = 8
	maxPathLen       = 200
)

var skillIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*$`)

// roleOverride is `.nightshift/roles/<role>.yaml` of the project (design section 6): extra
// instruction files and extra skills for one role. It only adds. The output schema, limits,
// access and the gates stay the factory's, so a project cannot weaken a role, only brief it.
type roleOverride struct {
	Instructions []string `yaml:"instructions"` // files of the repository
	Skills       []string `yaml:"skills"`       // ids from the shared skill set
}

// withRoleOverride returns the job's instruction files and skills plus the project's additions
// for the role. A missing file changes nothing. A broken one (unknown key, bad path or id, too
// many items, a file that is not in the repository) is skipped whole with a warning: the role
// then runs as the factory defines it, which is never weaker.
func (c *Conductor) withRoleOverride(ctx context.Context, mirror, base, role string, instructions, skills []string) (instr, sk []string) {
	doc, ok, err := c.Git.Show(ctx, mirror, base, ".nightshift/roles/"+role+".yaml")
	if err != nil || !ok {
		return instructions, skills
	}
	o, err := parseRoleOverride(doc)
	if err == nil {
		for _, p := range o.Instructions {
			if ok, serr := c.Git.IsRegularFile(ctx, mirror, base, p); serr != nil || !ok {
				err = &overrideError{"instruction file " + p + " is not a regular file of the repository"}
				break
			}
		}
	}
	if err != nil {
		c.log().WarnContext(ctx, "role override skipped", "role", role, "error", err)
		return instructions, skills
	}
	for _, p := range o.Instructions {
		if !slices.Contains(instructions, p) {
			instructions = append(slices.Clone(instructions), p)
		}
	}
	for _, id := range o.Skills {
		if !slices.Contains(skills, id) {
			skills = append(slices.Clone(skills), id)
		}
	}
	return instructions, skills
}

type overrideError struct{ msg string }

func (e *overrideError) Error() string { return e.msg }

func parseRoleOverride(doc string) (roleOverride, error) {
	var o roleOverride
	dec := yaml.NewDecoder(bytes.NewReader([]byte(doc)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil && err.Error() != "EOF" {
		return roleOverride{}, err
	}
	if len(o.Instructions) > maxOverrideItems || len(o.Skills) > maxOverrideItems {
		return roleOverride{}, &overrideError{"more than 8 instruction files or skills"}
	}
	for _, p := range o.Instructions {
		if p == "" || len(p) > maxPathLen || path.IsAbs(p) || path.Clean(p) != p || strings.HasPrefix(p, "..") || strings.HasPrefix(p, "-") {
			return roleOverride{}, &overrideError{"bad instruction path " + p}
		}
	}
	for _, id := range o.Skills {
		if !skillIDRe.MatchString(id) {
			return roleOverride{}, &overrideError{"bad skill id " + id}
		}
	}
	return o, nil
}
