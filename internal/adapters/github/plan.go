package github

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/ilyachakun/nightshift/internal/integration"
)

var _ integration.PlanLog = (*Client)(nil)

// maxPlanFiles bounds the files read from the plan directory.
const maxPlanFiles = 20

var (
	planDone  = regexp.MustCompile(`(?m)^\s*- \[[xX]\]`)
	planOpen  = regexp.MustCompile(`(?m)^\s*- \[ \]`)
	planPhase = regexp.MustCompile(`(?m)^# +(Phase \d+)\b`)
)

// PlanProgress reads the markdown files under docs/plan of the repository and counts their
// checklist items, done and in all. A repository without that directory has no plan: an empty
// list and no error. A file headed "Phase N" is named so; any other is named by its file name, which is unique.
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
		if len(out) == maxPlanFiles {
			break
		}
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
	done := len(planDone.FindAllString(text, -1))
	p := integration.PlanProgress{Name: file, Done: done, Total: done + len(planOpen.FindAllString(text, -1))}
	if m := planPhase.FindStringSubmatch(text); m != nil {
		p.Name = m[1]
	}
	return p
}
