package skills

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	maxFile  = 1 << 20
	maxTotal = 8 << 20
)

// Options of Sync. Paths are plain directories, not an fs.FS, because Sync writes.
type Options struct {
	File       string // skills/vendor.yaml
	Dir        string // skills/vendor
	DryRun     bool   // report only; write nothing
	AllowLocal bool   // accept a local path as repo (tests)
}

// Result is what Sync found for one entry.
type Result struct {
	ID       string
	Diff     string   // unified diff against what is in Dir; empty when nothing changes
	Findings []string // why the entry was refused
	Notes    []string
}

// Sync fetches every entry by SHA, scans it and, only when every entry is clean, replaces
// <Dir>/<id>. It never writes a refused skill. Entries missing from the file are reported,
// not deleted.
func Sync(ctx context.Context, o Options) ([]Result, error) {
	entries, err := Load(o.File, o.AllowLocal)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "skills-sync-")
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	var results []Result
	clean := true
	for _, e := range entries {
		r := Result{ID: e.ID}
		files, err := fetch(ctx, e)
		if err != nil {
			return nil, fmt.Errorf("skills: %s: %w", e.ID, err)
		}
		r.Findings = scan(files)
		meta, _ := yaml.Marshal(map[string]string{"repo": e.Repo, "path": e.Path, "sha": e.SHA, "license": e.License})
		files = append(files, file{Name: "VENDORED.yaml", Data: meta})
		stage := filepath.Join(tmp, "new", filepath.FromSlash(e.ID))
		if err := writeFiles(stage, files); err != nil {
			return nil, err
		}
		if msg, ran := leaks(ctx, stage); msg != "" {
			r.Findings = append(r.Findings, msg)
		} else if !ran {
			r.Notes = append(r.Notes, "gitleaks is not installed: secrets were not scanned")
		}
		old := filepath.Join(tmp, "old", filepath.FromSlash(e.ID))
		if err := os.MkdirAll(old, 0o755); err != nil {
			return nil, fmt.Errorf("skills: %w", err)
		}
		if err := copyTree(filepath.Join(o.Dir, filepath.FromSlash(e.ID)), old); err != nil {
			return nil, err
		}
		if r.Diff, err = diff(ctx, tmp, e.ID); err != nil {
			return nil, err
		}
		clean = clean && len(r.Findings) == 0
		results = append(results, r)
	}
	if clean && !o.DryRun {
		for _, r := range results {
			dst := filepath.Join(o.Dir, filepath.FromSlash(r.ID))
			if r.Diff == "" {
				continue
			}
			if err := os.RemoveAll(dst); err != nil {
				return nil, fmt.Errorf("skills: %w", err)
			}
			if err := copyTree(filepath.Join(tmp, "new", filepath.FromSlash(r.ID)), dst); err != nil {
				return nil, err
			}
		}
	}
	return results, orphans(o.Dir, entries, &results)
}

// Refused reports whether any entry has findings.
func Refused(rs []Result) bool {
	for _, r := range rs {
		if len(r.Findings) > 0 {
			return true
		}
	}
	return false
}

// Format renders the report the owner reads before approving the pull request.
func Format(rs []Result) string {
	var b strings.Builder
	for _, r := range rs {
		status := "unchanged"
		switch {
		case len(r.Findings) > 0:
			status = "REFUSED"
		case r.Diff != "":
			status = "changed"
		}
		fmt.Fprintf(&b, "## %s: %s\n", r.ID, status)
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "- finding: %s\n", f)
		}
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "- note: %s\n", n)
		}
		if r.Diff != "" {
			b.WriteString("\n```diff\n" + r.Diff + "```\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// fetch gets one skill folder at its SHA through git, so any host git can reach works and
// the SHA is verified by git itself.
func fetch(ctx context.Context, e Entry) ([]file, error) {
	dir, err := os.MkdirTemp("", "skills-git-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}
		return out, nil
	}
	for _, args := range [][]string{{"init", "-q"}, {"fetch", "-q", "--depth=1", "--no-tags", "--", e.Repo, e.SHA}} {
		if _, err := git(args...); err != nil {
			return nil, err
		}
	}
	if head, err := git("rev-parse", "FETCH_HEAD"); err != nil || strings.TrimSpace(string(head)) != e.SHA {
		return nil, fmt.Errorf("fetched commit is not %s", e.SHA)
	}
	archive, err := git("archive", "--format=tar", "FETCH_HEAD:"+e.Path)
	if err != nil {
		return nil, err
	}
	files, err := untar(archive)
	if err != nil {
		return nil, err
	}
	if !hasFile(files, "SKILL.md") {
		return nil, errors.New("no SKILL.md in the folder")
	}
	if !hasLicense(files) {
		root, err := git("ls-tree", "--name-only", "FETCH_HEAD")
		if err != nil {
			return nil, err
		}
		for _, n := range strings.Fields(string(root)) {
			if isLicense(n) {
				data, err := git("cat-file", "blob", "FETCH_HEAD:"+n)
				if err != nil {
					return nil, err
				}
				return append(files, file{Name: "LICENSE", Data: data}), nil
			}
		}
		return nil, errors.New("no licence file in the folder or at the repository root")
	}
	return files, nil
}

func untar(data []byte) ([]file, error) {
	var files []file
	total := 0
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		switch h.Typeflag {
		case tar.TypeDir, tar.TypeXGlobalHeader:
			continue
		case tar.TypeReg:
		default: // symlinks and links can point outside the skill
			files = append(files, file{Name: h.Name, Link: true})
			continue
		}
		if h.Size > maxFile || total+int(h.Size) > maxTotal {
			return nil, fmt.Errorf("%s: the skill is larger than %d MiB", h.Name, maxTotal>>20)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		total += len(b)
		files = append(files, file{Name: h.Name, Exec: h.Mode&0o111 != 0, Data: b})
	}
}

func hasFile(fs []file, name string) bool {
	for _, f := range fs {
		if f.Name == name {
			return true
		}
	}
	return false
}

func hasLicense(fs []file) bool {
	for _, f := range fs {
		if !strings.Contains(f.Name, "/") && isLicense(f.Name) {
			return true
		}
	}
	return false
}

func isLicense(n string) bool {
	n = strings.ToUpper(n)
	return strings.HasPrefix(n, "LICENSE") || strings.HasPrefix(n, "LICENCE") || strings.HasPrefix(n, "COPYING")
}

// writeFiles never keeps an executable bit; names were already checked by tar and git.
func writeFiles(dir string, files []file) error {
	for _, f := range files {
		if f.Link {
			continue // refused by scan; never written
		}
		p := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && p == src {
				return nil
			}
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

// diff compares old/<id> with new/<id> under root with git, so the owner reads a normal diff.
func diff(ctx context.Context, root, id string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-c", "diff.noprefix=true", "diff", "--no-index", "--no-color", "--", "old/"+id, "new/"+id)
	cmd.Dir = root
	out, err := cmd.Output()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) { // exit 1 means "differs"
		return "", fmt.Errorf("skills: git diff: %w", err)
	}
	return string(out), nil
}

func orphans(dir string, entries []Entry, rs *[]Result) error {
	known := map[string]bool{}
	for _, e := range entries {
		known[e.ID] = true
	}
	sets, err := os.ReadDir(dir)
	if err != nil {
		return nil //nolint:nilerr // no vendor folder yet: nothing is orphaned
	}
	var extra []string
	for _, s := range sets {
		names, _ := os.ReadDir(filepath.Join(dir, s.Name()))
		for _, n := range names {
			if id := s.Name() + "/" + n.Name(); s.IsDir() && !known[id] {
				extra = append(extra, id)
			}
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		*rs = append(*rs, Result{ID: id, Notes: []string{"vendored but not in vendor.yaml; delete it in the same pull request"}})
	}
	return nil
}
