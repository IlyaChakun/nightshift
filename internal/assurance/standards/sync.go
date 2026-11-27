package standards

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Card is the part of a standard that `standards list` shows.
type Card struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Enforcement string `yaml:"enforcement"`
	Severity    string `yaml:"severity"`
	Status      string `yaml:"status"`
	Stack       string `yaml:"-"` // the folder the card sits in: a stack or a discipline
}

// List returns the cards of the given folders (stacks or disciplines; none means all), in path order.
func List(fsys fs.FS, folders []string) ([]Card, error) {
	var cards []Card
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".yaml" || path.Base(path.Dir(p)) == "files" {
			return err
		}
		folder := path.Base(path.Dir(p))
		if len(folders) > 0 && !slices.Contains(folders, folder) {
			return nil
		}
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		var c Card
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		c.Stack = folder
		cards = append(cards, c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("standards: %w", err)
	}
	return cards, nil
}

// Sync copies the tool configuration files of the stacks (`<set>/<stack>/files/<path>`) into
// the project at dest. A file the project already has is never overwritten: the project's own
// configuration wins, and the caller opens a pull request for what was written (design section 9).
func Sync(fsys fs.FS, stacks []string, dest string) (written, kept []string, err error) {
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, ok := configPath(p, stacks)
		if !ok {
			return nil
		}
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if _, statErr := os.Stat(target); statErr == nil {
			kept = append(kept, rel)
			return nil
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil { //nolint:gosec // a project file that is committed, not a secret
			return err
		}
		written = append(written, rel)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("standards: %w", err)
	}
	return written, kept, nil
}

// configPath returns the project path of `<set>/<stack>/files/<path>` when the stack is wanted.
func configPath(p string, stacks []string) (string, bool) {
	parts := strings.Split(p, "/")
	for i := 1; i+1 < len(parts); i++ {
		if parts[i] == "files" && slices.Contains(stacks, parts[i-1]) {
			return strings.Join(parts[i+1:], "/"), true
		}
	}
	return "", false
}
