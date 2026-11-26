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
	planTitle = regexp.MustCompile(`(?m)^# +(.+)$`)
)

// PlanProgress reads the markdown files under docs/plan of the repository and counts their
// checklist items, done and in all. A repository without that directory has no plan: an empty
// list and no error. The file's first heading, up to a dash, is its name.
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
		if f.Type != "file" || !strings.HasSuffix(f.Name, ".md") {
			continue
		}
		if len(out) == maxPlanFiles {
			break
		}
		var doc struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if err := c.do(ctx, http.MethodGet, base+"/contents/"+f.Path, nil, &doc); err != nil {
			return out, err
		}
		if doc.Encoding != "base64" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(doc.Content, "\n", ""))
		if err != nil {
			continue
		}
		out = append(out, countPlan(strings.TrimSuffix(f.Name, ".md"), string(raw)))
	}
	return out, nil
}

// countPlan counts the checklist items of one plan file.
func countPlan(file, text string) integration.PlanProgress {
	done := len(planDone.FindAllString(text, -1))
	p := integration.PlanProgress{Name: file, Done: done, Total: done + len(planOpen.FindAllString(text, -1))}
	if m := planTitle.FindStringSubmatch(text); m != nil {
		name, _, _ := strings.Cut(m[1], " — ")
		p.Name = strings.TrimSpace(name)
	}
	return p
}
