// Package codehost is the code host as the factory uses it (design sections 7 and 11): pull
// requests, CI, reviews and commit statuses. Dispatch, the merge train and onboarding consume
// it; the GitHub adapter implements it.
package codehost

import "context"

// PullRequests is the code host as the factory uses it (design sections 7 and 11): open a
// draft PR from a branch nightshift pushed, hand it to people, read CI and reviews back, and
// publish the `nightshift/human-review` status. Merging is added with the merge train.
type PullRequests interface {
	OpenPR(ctx context.Context, pr OpenPR) (PRRef, error)
	PRState(ctx context.Context, ref PRRef) (PRState, error)
	MarkReady(ctx context.Context, ref PRRef) error
	// Merge squashes the pull request when its head is still ref.HeadSHA and returns the
	// merge commit; a moved head is an error, never a merge of unseen code.
	Merge(ctx context.Context, ref PRRef) (string, error)
	RequestReviewers(ctx context.Context, ref PRRef, users []string) error
	AddLabels(ctx context.Context, ref PRRef, labels []string) error
	SetStatus(ctx context.Context, repo, sha string, st CommitStatus) error
	// CI summarizes the checks and statuses of a commit, without the factory's own
	// `nightshift/` statuses, so the factory does not wait for itself.
	CI(ctx context.Context, repo, sha string) (CIResult, error)
	Reviews(ctx context.Context, ref PRRef) ([]Review, error)
	// LineComments renders the comments one review left on lines of the diff. Ask it only for
	// a review of a person who may decide: anyone can review a public repository.
	LineComments(ctx context.Context, ref PRRef, review int64) (string, error)
}

// PRRef points to a pull request at a given head.
type PRRef struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	HeadSHA string `json:"head_sha"`
}

// OpenPR is a request for a pull request. Head is a branch nightshift pushed.
type OpenPR struct {
	Repo  string `json:"repo"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

// PRState is what the factory needs to know about a pull request.
type PRState struct {
	Ref    PRRef  `json:"ref"`
	State  string `json:"state"` // open, closed or merged
	Draft  bool   `json:"draft"`
	URL    string `json:"url"`
	Merged bool   `json:"merged"`
	// MergeSHA is the commit the merge made on the base branch, when the pull request is merged.
	// A release deploys it, never the head of the branch.
	MergeSHA string `json:"merge_sha,omitempty"`
	// MergedBy is the login that merged the pull request, when the code host says.
	MergedBy string `json:"merged_by,omitempty"`
}

// CommitStatus is a status on a commit; State is pending, success or failure.
type CommitStatus struct {
	Context     string `json:"context"`
	State       string `json:"state"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// CIState is the summary of the checks of a commit.
type CIState string

// CI summaries. Pending means something still runs or nothing has reported yet.
const (
	CIPending CIState = "pending"
	CISuccess CIState = "success"
	CIFailure CIState = "failure"
)

// CIResult is the CI summary and the names of the failing checks.
type CIResult struct {
	State   CIState  `json:"state"`
	Failing []string `json:"failing,omitempty"`
	// Stable identities for checks the forge can match across heads.
	FailedChecks []string `json:"failed_checks,omitempty"`
	PassedChecks []string `json:"passed_checks,omitempty"`
	// Detail is what the failing checks say about themselves (their summary and the messages
	// they put on lines), bounded; empty when the code host gave nothing. Full job logs are not
	// read: they come from a second host.
	Detail string `json:"detail,omitempty"`
}

// Review is one review of a pull request at the commit it was made on.
type Review struct {
	User      string `json:"user"`
	State     string `json:"state"` // approved, changes_requested, commented or dismissed
	CommitSHA string `json:"commit_sha"`
	ID        int64  `json:"id"`
	Body      string `json:"body"` // the reviewer's summary
}
