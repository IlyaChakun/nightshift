// Package skills vendors external skills by commit SHA (design section 12): it reads
// skills/vendor.yaml, fetches each entry, scans it for obvious injection and reports what
// changed. It writes files and nothing else; opening the pull request and the owner's Approve
// are the caller's job.
package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	idRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*$`)
	shaRe = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Entry is one line of skills/vendor.yaml.
type Entry struct {
	ID        string         `yaml:"id"`
	Repo      string         `yaml:"repo"`
	Path      string         `yaml:"path"`
	SHA       string         `yaml:"sha"`
	License   string         `yaml:"license"`
	Roles     []string       `yaml:"roles"`
	AppliesTo map[string]any `yaml:"applies_to"`
}

// Load reads and validates the vendor file. An unknown key is an error, so a typo cannot
// silently drop a pin.
func Load(file string, allowLocal bool) ([]Entry, error) {
	f, err := os.Open(file) //nolint:gosec // the path is the command line flag
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	defer func() { _ = f.Close() }()
	var entries []Entry
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&entries); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("skills: parse %s: %w", file, err)
	}
	return entries, Validate(entries, allowLocal)
}

// Validate checks every entry and reports all problems at once. allowLocal accepts an
// absolute local path as the repository; production entries must be https URLs.
func Validate(entries []Entry, allowLocal bool) error {
	var errs []error
	seen := map[string]bool{}
	for i, e := range entries {
		bad := func(format string, a ...any) {
			errs = append(errs, fmt.Errorf("entry %d (%q): %s", i+1, e.ID, fmt.Sprintf(format, a...)))
		}
		switch {
		case !idRe.MatchString(e.ID):
			bad("id must look like <set>/<name> in lower case")
		case strings.HasPrefix(e.ID, "own/"):
			bad("the set own/ is reserved for the factory's own skills")
		case seen[e.ID]:
			bad("duplicate id")
		}
		seen[e.ID] = true
		if !shaRe.MatchString(e.SHA) {
			bad("sha must be 40 lower-case hex characters, not a branch or a tag")
		}
		if strings.TrimSpace(e.License) == "" {
			bad("license is missing")
		}
		if !strings.HasPrefix(e.Repo, "https://") && (!allowLocal || !filepath.IsAbs(e.Repo)) {
			bad("repo must be an https URL")
		}
		if !fs.ValidPath(e.Path) || e.Path == "." {
			bad("path must be a relative folder inside the repository")
		}
		if len(e.Roles) == 0 {
			bad("roles is empty")
		}
	}
	return errors.Join(errs...)
}
