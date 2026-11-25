package standards

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Levels in order of precedence, strongest first (design section 9): factory rules on
// security and gates, then the project, then the house, then the industry.
var Levels = []string{"factory", "project", "house", "industry"}

// Rule is one rule card. Values of Params are the declarations in a general card and the
// project's own values in an overriding card.
type Rule struct {
	ID          string         `yaml:"id"`
	Title       string         `yaml:"title"`
	AppliesTo   map[string]any `yaml:"applies_to"`
	Enforcement string         `yaml:"enforcement"`
	Tool        map[string]any `yaml:"tool"`
	Rubric      *string        `yaml:"rubric"`
	Params      map[string]any `yaml:"params"`
	Severity    string         `yaml:"severity"`
	Status      string         `yaml:"status"`
	Source      string         `yaml:"source"`
	Reason      string         `yaml:"reason"`
	Level       string         `yaml:"-"`
	Path        string         `yaml:"-"`
	Effective   map[string]any `yaml:"-"`
}

// Param declares one value a project may set.
type Param struct {
	Type     string `yaml:"type"` // string, int, bool
	Enum     []any  `yaml:"enum"`
	Stricter string `yaml:"stricter"` // lower or higher: the direction that tightens the rule
	Default  any    `yaml:"default"`
}

// Resolved is the library after precedence: one rule per id, with its effective params.
type Resolved struct {
	Rules []Rule
	// Weakened lists "id.param" values a project set in the weaker direction; the owner approves them.
	Weakened []string
}

// LoadRules reads every `*.yaml` rule card under dir/<level>/ for the levels that exist.
// A level directory that is missing is empty. Files without an `id` are not cards (a pack's
// other YAML) and are skipped.
func LoadRules(fsys fs.FS) ([]Rule, error) {
	var out []Rule
	for _, level := range Levels {
		sub, err := fs.Sub(fsys, level)
		if err != nil {
			return nil, fmt.Errorf("standards: %w", err)
		}
		err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d == nil || p == "." {
					return nil // the level directory is missing
				}
				return err
			}
			if d.IsDir() || path.Ext(p) != ".yaml" || strings.Contains(p, "/files/") {
				return nil
			}
			raw, err := fs.ReadFile(sub, p)
			if err != nil {
				return err
			}
			var c Rule
			if err := yaml.Unmarshal(raw, &c); err != nil {
				return fmt.Errorf("%s/%s: %w", level, p, err)
			}
			if c.ID == "" {
				return nil
			}
			c.Level, c.Path = level, level+"/"+p
			out = append(out, c)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("standards: %w", err)
		}
	}
	return out, nil
}

// Resolve applies precedence. When two levels hold a card with the same id the stronger one
// wins; a card that overrides a more general one needs a `reason`, and nothing overrides a
// factory card. Params of the stronger card are checked against the declarations of the
// general one.
func Resolve(cards []Rule) (Resolved, error) {
	rank := map[string]int{}
	for i, l := range Levels {
		rank[l] = i
	}
	byID := map[string][]Rule{}
	for _, c := range cards {
		byID[c.ID] = append(byID[c.ID], c)
	}
	var res Resolved
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		group := byID[id]
		sort.SliceStable(group, func(i, j int) bool { return rank[group[i].Level] < rank[group[j].Level] })
		win := group[0]
		for _, general := range group[1:] {
			if general.Level == win.Level {
				return Resolved{}, fmt.Errorf("standards: %s is defined twice at level %s (%s, %s)", id, win.Level, win.Path, general.Path)
			}
			if general.Level == "factory" {
				return Resolved{}, fmt.Errorf("standards: %s overrides a factory rule: %s", win.Path, general.Path)
			}
		}
		if len(group) > 1 && strings.TrimSpace(win.Reason) == "" {
			return Resolved{}, fmt.Errorf("standards: %s overrides %s and needs a `reason`", win.Path, group[1].Path)
		}
		if len(group) > 1 {
			weak, err := applyParams(&win, group[1])
			if err != nil {
				return Resolved{}, err
			}
			res.Weakened = append(res.Weakened, weak...)
		}
		res.Rules = append(res.Rules, win)
	}
	return res, nil
}

// applyParams checks the project's values against the declarations and sets Effective.
func applyParams(win *Rule, general Rule) (weakened []string, err error) {
	win.Effective = map[string]any{}
	for name, raw := range general.Params {
		var decl Param
		b, _ := yaml.Marshal(raw)
		if err := yaml.Unmarshal(b, &decl); err != nil {
			return nil, fmt.Errorf("standards: %s: param %s: %w", general.Path, name, err)
		}
		win.Effective[name] = decl.Default
		val, set := win.Params[name]
		if !set {
			continue
		}
		if err := decl.check(val); err != nil {
			return nil, fmt.Errorf("standards: %s: param %s: %w", win.Path, name, err)
		}
		win.Effective[name] = val
		if decl.weaker(val) {
			weakened = append(weakened, win.ID+"."+name)
		}
	}
	for name := range win.Params {
		if _, ok := general.Params[name]; !ok {
			return nil, fmt.Errorf("standards: %s sets %s, which %s does not declare", win.Path, name, general.Path)
		}
	}
	return weakened, nil
}

func (p Param) check(v any) error {
	ok := false
	switch p.Type {
	case "string":
		_, ok = v.(string)
	case "int":
		_, ok = v.(int)
	case "bool":
		_, ok = v.(bool)
	default:
		return fmt.Errorf("unknown type %q", p.Type)
	}
	if !ok {
		return fmt.Errorf("%v is not of type %s", v, p.Type)
	}
	if len(p.Enum) > 0 {
		for _, e := range p.Enum {
			if e == v {
				return nil
			}
		}
		return fmt.Errorf("%v is not one of %v", v, p.Enum)
	}
	return nil
}

// weaker reports a number moved against the stricter direction. Other types have no direction.
func (p Param) weaker(v any) bool {
	got, ok1 := v.(int)
	def, ok2 := p.Default.(int)
	if !ok1 || !ok2 {
		return false
	}
	switch p.Stricter {
	case "lower":
		return got > def
	case "higher":
		return got < def
	}
	return false
}
