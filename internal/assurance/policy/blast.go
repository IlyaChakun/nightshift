package policy

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// Level is how dangerous a change is (design section 11). The order is meaningful.
type Level string

// Levels, lowest first.
const (
	Low      Level = "low"
	Medium   Level = "medium"
	High     Level = "high"
	Critical Level = "critical"
)

func (l Level) rank() int { return slices.Index([]Level{Low, Medium, High, Critical}, l) }

// Valid reports whether l is one of the four levels.
func (l Level) Valid() bool { return l.rank() >= 0 }

// Max returns the higher of two levels; an unknown level counts as the lowest.
func Max(a, b Level) Level {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Signals are facts about a change that a path list cannot show.
type Signals struct {
	NewDependency bool `yaml:"new_dependency"`
	NewModule     bool `yaml:"new_module"`
	DiffLinesOver int  `yaml:"diff_lines_over"`
}

// LevelRule is what puts a change on one level.
type LevelRule struct {
	Paths        []string `yaml:"paths"`
	Tags         []string `yaml:"tags"`
	Signals      Signals  `yaml:"signals"`
	MaxDiffLines int      `yaml:"max_diff_lines"`
	Overridable  *bool    `yaml:"overridable"`
}

// Rules is .nightshift/blast-radius.yaml, compiled.
type Rules struct {
	rules map[Level]LevelRule
	paths map[Level][]*regexp.Regexp
}

// ParseRules validates the file against schemas/blast-radius.json and compiles its globs.
func ParseRules(doc []byte, v *config.Validator) (*Rules, error) {
	if err := v.ValidateYAML(config.KindBlastRadius, doc); err != nil {
		return nil, fmt.Errorf("policy: blast radius rules: %w", err)
	}
	var f struct {
		Levels map[Level]LevelRule `yaml:"levels"`
	}
	if err := yaml.Unmarshal(doc, &f); err != nil {
		return nil, fmt.Errorf("policy: blast radius rules: %w", err)
	}
	r := &Rules{rules: f.Levels, paths: map[Level][]*regexp.Regexp{}}
	for lvl, rule := range f.Levels {
		for _, g := range rule.Paths {
			re, err := compileGlob(g)
			if err != nil {
				return nil, err
			}
			r.paths[lvl] = append(r.paths[lvl], re)
		}
	}
	return r, nil
}

// Change is what the classifier looks at. It comes from the patch and the task, never from
// an agent's claim about its own safety.
type Change struct {
	Paths         []string
	DiffLines     int
	NewDependency bool
	NewModule     bool
	// Tags are set by Lead when it creates the task, for example "payments".
	Tags []string
	// Floor is the level an agent asked for; it can only raise the result.
	Floor Level
	// Privileged are paths that a privileged service runs or reads (ProjectExecuted). They count
	// with the built-in protected paths as critical, whatever the project's rules say.
	Privileged []string
}

// Classification is the level and the reasons that led to it.
type Classification struct {
	Level   Level
	Reasons []string
}

type hit struct {
	level Level
	why   string
}

// Classify returns the highest level any signal reaches. Built-in protected paths and the
// change's privileged paths are always critical. Anything unmatched is medium, and
// low needs every changed path on a low path and a small diff. The reasons are those of the
// level that won.
func (r *Rules) Classify(c Change) Classification {
	var hits []hit
	for _, l := range []Level{High, Critical} {
		hits = append(hits, r.hits(l, c)...)
	}
	if c.Floor.Valid() {
		hits = append(hits, hit{c.Floor, "raised by an agent"})
	}
	if len(hits) == 0 && r.isLow(c) {
		return Classification{Level: Low, Reasons: []string{"only low paths and a small diff"}}
	}
	level := Medium
	for _, h := range hits {
		level = Max(level, h.level)
	}
	var reasons []string
	for _, h := range hits {
		if h.level == level && !slices.Contains(reasons, h.why) {
			reasons = append(reasons, h.why)
		}
	}
	if len(reasons) == 0 {
		reasons = []string{"nothing else matched"}
	}
	return Classification{Level: level, Reasons: reasons}
}

// hits lists what in the change reaches level l.
func (r *Rules) hits(l Level, c Change) []hit {
	var out []hit
	add := func(why string) { out = append(out, hit{l, why}) }
	rule := r.rules[l]
	for _, p := range c.Paths {
		if r.matches(l, p) {
			add("path " + p)
		}
	}
	if l == Critical {
		out = append(out, privilegedHits(c)...)
	}
	for _, t := range rule.Tags {
		if slices.Contains(c.Tags, t) {
			add("tag " + t)
		}
	}
	if rule.Signals.NewDependency && c.NewDependency {
		add("new dependency")
	}
	if rule.Signals.NewModule && c.NewModule {
		add("new module")
	}
	if n := rule.Signals.DiffLinesOver; n > 0 && c.DiffLines > n {
		add(fmt.Sprintf("diff of %d lines", c.DiffLines))
	}
	return out
}

// privilegedHits finds the changed paths that are built-in protected or run by a privileged
// service. A list that does not compile is a hit of its own: nothing is judged on a rule that
// cannot be read.
func privilegedHits(c Change) []hit {
	con, err := NewConstitution(c.Privileged)
	if err != nil {
		return []hit{{Critical, "privileged paths unreadable"}}
	}
	var out []hit
	for _, p := range con.Hits(c.Paths) {
		out = append(out, hit{Critical, "path " + p})
	}
	return out
}

func (r *Rules) matches(l Level, p string) bool {
	return slices.ContainsFunc(r.paths[l], func(re *regexp.Regexp) bool { return re.MatchString(strings.TrimPrefix(p, "./")) })
}

func (r *Rules) isLow(c Change) bool {
	rule, ok := r.rules[Low]
	if !ok || len(c.Paths) == 0 {
		return false
	}
	if rule.MaxDiffLines > 0 && c.DiffLines > rule.MaxDiffLines {
		return false
	}
	return !slices.ContainsFunc(c.Paths, func(p string) bool { return !r.matches(Low, p) })
}
