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

// maxPlanFiles bounds the parts read from the plan directory.
const maxPlanFiles = 20

var (
	planDone  = regexp.MustCompile(`(?m)^\s*- \[[xX]\]`)
	planOpen  = regexp.MustCompile(`(?m)^\s*- \[ \]`)
	planPhase = regexp.MustCompile(`(?m)^# +(Phase \d+)\b`)
)

// PlanProgress reads the plan under docs/plan of the repository and counts its checklist items,
// done and in all. A part of the plan is a markdown file, or a folder with a README.md that holds
// the items; other files of a folder are notes and proposals and are not counted. A repository
// without that directory has no plan: an empty list and no error. A part headed "Phase N" is
// named so; any other is named by its file or folder name, which is unique.
func (c *Client) PlanProgress(ctx context.Context, repo string) ([]integration.PlanProgress, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/contents/docs/plan", nil, &entries); err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []integration.PlanProgress
	for _, e := range entries {
		name, path := strings.TrimSuffix(e.Name, ".md"), e.Path
		switch {
		case e.Type == "dir":
			path += "/README.md"
		case e.Type != "file" || !strings.HasSuffix(e.Name, ".md"):
			continue
		}
		if len(out) == maxPlanFiles {
			break
		}
		var doc struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := c.do(ctx, http.MethodGet, base+"/contents/"+path, nil, &doc); err != nil {
			if e.Type == "dir" && isNotFound(err) {
				continue // a folder without a README.md is not a part of the plan
			}
			return out, err
		}
		if doc.Encoding != "base64" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(doc.Content, "\n", ""))
		if err != nil {
			continue
		}
		out = append(out, countPlan(name, string(raw)))
	}
	return out, nil
}

func isNotFound(err error) bool {
	var he *Error
	return errors.As(err, &he) && he.Status == http.StatusNotFound
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
