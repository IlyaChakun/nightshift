package github

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

var _ integration.MergeLog = (*Client)(nil)

// maxDigestPages bounds the listing: 500 pull requests closed since the window opened is
// more than any night produces, and the digest says so when it stops there.
const maxDigestPages = 5

var (
	whyHeading = regexp.MustCompile(`(?im)^#{1,4}\s*what and why\s*$`)
	reviewedBy = regexp.MustCompile(`(?i)reviewed by`)
)

type mergedDoc struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	HTMLURL   string     `json:"html_url"`
	Body      string     `json:"body"`
	MergedAt  *time.Time `json:"merged_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	MergedBy  *struct {
		Login string `json:"login"`
	} `json:"merged_by"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// MergedSince lists the pull requests merged after since, oldest first. The list endpoint
// gives the merger on a pull request only in the single-PR view, so MergedBy may be empty.
func (c *Client) MergedSince(ctx context.Context, repo string, since time.Time) ([]integration.MergedPR, error) {
	base, err := repoPath(repo)
	if err != nil {
		return nil, err
	}
	var out []integration.MergedPR
	for page := 1; page <= maxDigestPages; page++ {
		var docs []mergedDoc
		path := base + "/pulls?state=closed&sort=updated&direction=desc&per_page=100&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, path, nil, &docs); err != nil {
			return nil, err
		}
		older := false
		for _, d := range docs {
			if d.UpdatedAt.Before(since) {
				older = true // sorted by update time: nothing later in the list can be newer
				continue
			}
			if d.MergedAt == nil || d.MergedAt.Before(since) {
				continue
			}
			m := d.merged()
			out = append(out, m)
		}
		if older || len(docs) < 100 {
			break
		}
	}
	slices.SortFunc(out, func(a, b integration.MergedPR) int { return a.MergedAt.Compare(b.MergedAt) })
	return out, nil
}

// reason is the first paragraph under "What and why", cut to 240 characters.
func reason(body string) string {
	loc := whyHeading.FindStringIndex(body)
	if loc == nil {
		return ""
	}
	rest := strings.TrimSpace(body[loc[1]:])
	para, _, _ := strings.Cut(rest, "\n\n")
	para = strings.Join(strings.Fields(para), " ")
	if r := []rune(para); len(r) > 240 {
		para = string(r[:240]) + "..."
	}
	return para
}

func (d mergedDoc) merged() integration.MergedPR {
	m := integration.MergedPR{
		Number: d.Number, Title: d.Title, URL: d.HTMLURL, MergedAt: *d.MergedAt,
		Reason: reason(d.Body), Reviewed: reviewedBy.MatchString(d.Body),
	}
	if d.MergedBy != nil {
		m.MergedBy = d.MergedBy.Login
	}
	for _, l := range d.Labels {
		m.Labels = append(m.Labels, l.Name)
	}
	return m
}
