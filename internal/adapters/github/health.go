package github

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/ilyachakun/nightshift/internal/observability/summary"
)

var _ summary.RepoHealth = (*Client)(nil)

// maxHealthPages bounds each listing: 300 open items is more than a healthy repository has.
const maxHealthPages = 3

// OpenAlerts lists the open code-scanning alerts. A token without the security-events read
// right gets an error here, which the digest shows as "unknown", not as "no alerts".
func (c *Client) OpenAlerts(ctx context.Context, repo string) ([]summary.Alert, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out []summary.Alert
	for page := 1; page <= maxHealthPages; page++ {
		var docs []struct {
			Number    int       `json:"number"`
			HTMLURL   string    `json:"html_url"`
			CreatedAt time.Time `json:"created_at"`
			Rule      struct {
				ID       string `json:"id"`
				Level    string `json:"severity"`
				Security string `json:"security_severity_level"`
			} `json:"rule"`
			Instance struct {
				Location struct {
					Path string `json:"path"`
				} `json:"location"`
			} `json:"most_recent_instance"`
		}
		if err := c.do(ctx, http.MethodGet, base+"/code-scanning/alerts?state=open&per_page=100&page="+strconv.Itoa(page), nil, &docs); err != nil {
			return out, err
		}
		for _, d := range docs {
			sev := d.Rule.Security
			if sev == "" {
				sev = d.Rule.Level
			}
			out = append(out, summary.Alert{Number: d.Number, Rule: d.Rule.ID, Severity: sev, Path: d.Instance.Location.Path, URL: d.HTMLURL, Created: d.CreatedAt})
		}
		if len(docs) < 100 {
			break
		}
	}
	return out, nil
}

// OpenPullRequests lists the open pull requests, least recently updated first.
func (c *Client) OpenPullRequests(ctx context.Context, repo string) ([]summary.PRActivity, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out []summary.PRActivity
	for page := 1; page <= maxHealthPages; page++ {
		var docs []struct {
			Number    int       `json:"number"`
			Title     string    `json:"title"`
			HTMLURL   string    `json:"html_url"`
			Draft     bool      `json:"draft"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := c.do(ctx, http.MethodGet, base+"/pulls?state=open&per_page=100&page="+strconv.Itoa(page), nil, &docs); err != nil {
			return out, err
		}
		for _, d := range docs {
			out = append(out, summary.PRActivity{Number: d.Number, Title: d.Title, URL: d.HTMLURL, Draft: d.Draft, Updated: d.UpdatedAt})
		}
		if len(docs) < 100 {
			break
		}
	}
	slices.SortFunc(out, func(a, b summary.PRActivity) int { return a.Updated.Compare(b.Updated) })
	return out, nil
}

// DefaultBranchFailures names the checks of the latest commit of main that failed or timed out.
// A canceled check is not a failure: a newer push cancels the run before it.
func (c *Client) DefaultBranchFailures(ctx context.Context, repo string) ([]string, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var doc struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/commits/main/check-runs?per_page=100", nil, &doc); err != nil {
		return nil, err
	}
	var out []string
	for _, r := range doc.CheckRuns {
		if r.Conclusion == "failure" || r.Conclusion == "timed_out" {
			out = append(out, r.Name)
		}
	}
	return out, nil
}
