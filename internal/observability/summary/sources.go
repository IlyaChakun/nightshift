package summary

import (
	"context"
	"time"
)

// MergeLog lists what was merged into a repository, for the morning digest (ADR 0013): the
// owner reads every merge of the night, whoever made it, with its reason and a way back. An
// error may come with a partial list; the caller shows both.
type MergeLog interface {
	MergedSince(ctx context.Context, repo string, since time.Time) ([]MergedPR, error)
}

// MergedPR is one merged pull request.
type MergedPR struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	MergedBy string    `json:"merged_by"`
	MergedAt time.Time `json:"merged_at"`
	Labels   []string  `json:"labels,omitempty"`
	// Reason is the "What and why" paragraph of the description, cut short.
	Reason string `json:"reason,omitempty"`
	// Reviewed is true when the description records an independent review ("Reviewed by").
	Reviewed bool `json:"reviewed"`
}

// PlanLog reads how far a repository's own plan has come, for the morning digest. A
// repository without a plan returns an empty list and no error.
type PlanLog interface {
	PlanProgress(ctx context.Context, repo string) ([]PlanProgress, error)
}

// PlanProgress is one part of the plan, a file or a folder: its name (the heading it opens with,
// else the name of the file or folder) and its checklist items, done and in all.
type PlanProgress struct {
	Name  string `json:"name"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// RepoHealth reads what a person would otherwise have to open the code host to see: open
// code-scanning alerts, open pull requests and the state of the default branch. The morning
// digest shows it (design section 10, factory health).
type RepoHealth interface {
	OpenAlerts(ctx context.Context, repo string) ([]Alert, error)
	OpenPullRequests(ctx context.Context, repo string) ([]PRActivity, error)
	// DefaultBranchFailures names the failing checks of the latest commit of the default branch.
	DefaultBranchFailures(ctx context.Context, repo string) ([]string, error)
}

// Alert is one open code-scanning alert.
type Alert struct {
	Repo     string    `json:"repo"` // owner/name; set by the reader of several repositories
	Number   int       `json:"number"`
	Rule     string    `json:"rule"`
	Severity string    `json:"severity"` // critical, high, medium, low, or the rule level when none is set
	Path     string    `json:"path"`
	URL      string    `json:"url"`
	Created  time.Time `json:"created"`
}

// PRActivity is one open pull request.
type PRActivity struct {
	Repo    string    `json:"repo"` // owner/name; set by the reader of several repositories
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Draft   bool      `json:"draft"`
	Updated time.Time `json:"updated"`
}
