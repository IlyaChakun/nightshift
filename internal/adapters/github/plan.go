package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/ilyachakun/nightshift/internal/integration"
)

var _ integration.PlanLog = (*Client)(nil)

// maxPlanFiles bounds the files read from the plan directory.
const maxPlanFiles = 20

var (
	planDone  = regexp.MustCompile(`(?m)^\s*- \[[xX]\]`)
	planOpen  = regexp.MustCompile(`(?m)^\s*- \[ \]`)
	planTitle = regexp.MustCompile(`^#\s+(\S.*?)\s*$`)
)

// PlanProgress reads the markdown files under docs/plan of the repository and counts their
// checklist items, done and in all. A repository without that directory has no plan: an empty
// list and no error. A part is named by the heading it opens with ("Phase 5 — Lead: planning and
// support"), which is what a person calls it; one without such a heading keeps the name of its
// file or folder.
func (c *Client) PlanProgress(ctx context.Context, repo string) ([]integration.PlanProgress, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var files []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/contents/docs/plan", nil, &files); err != nil {
		var he *Error
		if errors.As(err, &he) && he.Status == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	var out []integration.PlanProgress
	read := 0 // counts every fetch, so a directory full of folders without a README.md stays bounded
	for _, f := range files {
		// A part of the plan is a markdown file, or a folder whose README.md holds the items; the
		// other files of a folder are proposals and notes, and are not counted.
		path := f.Path
		switch {
		case f.Type == "dir":
			path += "/README.md"
		case f.Type != "file" || !strings.HasSuffix(f.Name, ".md"):
			continue
		}
		if read == maxPlanFiles {
			break
		}
		read++
		text, ok, err := c.planText(ctx, base+"/contents/"+path, f.Type == "dir")
		if err != nil {
			return out, err
		}
		if !ok {
			continue
		}
		out = append(out, countPlan(strings.TrimSuffix(f.Name, ".md"), text))
	}
	return out, nil
}

// planText fetches one plan file. It reports false for a file that cannot be decoded and, when
// optional is set, for one that does not exist: a folder without a README.md is not a part of the plan.
func (c *Client) planText(ctx context.Context, url string, optional bool) (string, bool, error) {
	var doc struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := c.do(ctx, http.MethodGet, url, nil, &doc); err != nil {
		var he *Error
		if optional && errors.As(err, &he) && he.Status == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	if doc.Encoding != "base64" {
		return "", false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(doc.Content, "\n", ""))
	return string(raw), err == nil, nil
}

// countPlan counts the checklist items of one plan file.
func countPlan(file, text string) integration.PlanProgress {
	text = strings.TrimPrefix(text, "\ufeff") // some editors put a byte order mark first; it hides the first line's heading or item
	done := len(planDone.FindAllString(text, -1))
	p := integration.PlanProgress{Name: file, Done: done, Total: done + len(planOpen.FindAllString(text, -1))}
	if title := planHeading(text); title != "" {
		p.Name = title
	}
	return p
}

// planHeading is the title of a plan file: a level-one heading on its first line with text,
// after front matter if there is any. A "# ..." line further down may be a comment in a code
// block, so it is not taken.
func planHeading(text string) string {
	lines := strings.Split(text, "\n")
	if strings.TrimSpace(lines[0]) == "---" {
		end := slices.IndexFunc(lines[1:], func(l string) bool { return strings.TrimSpace(l) == "---" })
		if end < 0 {
			return ""
		}
		lines = lines[end+2:]
	}
	for _, line := range lines {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if m := planTitle.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		break
	}
	return ""
}
