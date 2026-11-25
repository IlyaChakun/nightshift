// Package github is the code-host adapter: the REST calls the factory needs to hand work to
// people (design sections 7 and 11). It uses the standard library only. The caller gives it
// either a token read from the secrets directory or, better, a GitHub App (see App).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// DefaultBaseURL is the public API.
const DefaultBaseURL = "https://api.github.com"

var (
	repoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)
	shaRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// maxBody caps what is read from a response, so a broken server cannot flood memory. It holds
// a full page of reviews with the longest texts the code host allows, in any alphabet.
const maxBody = 32 << 20

// ownStatusPrefix marks the factory's own statuses; CI ignores them.
const ownStatusPrefix = "nightshift/"

// Client talks to the code host.
type Client struct {
	BaseURL string // defaults to DefaultBaseURL
	Token   string
	// TokenFor, when set, supplies the token of each repository instead of Token.
	TokenFor func(ctx context.Context, repo string) (string, error)
	HTTP     *http.Client // defaults to a client with a 30 second timeout
}

var _ integration.PullRequests = (*Client)(nil)

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// do sends one request about the repository its path names.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var repo string
	if parts := strings.SplitN(path, "/", 5); len(parts) >= 4 && parts[1] == "repos" {
		repo = parts[2] + "/" + parts[3]
		repo, _, _ = strings.Cut(repo, "?")
	}
	return c.send(ctx, repo, method, path, in, out)
}

func (c *Client) token(ctx context.Context, repo string) (string, error) {
	if c.TokenFor == nil || repo == "" {
		return c.Token, nil
	}
	return c.TokenFor(ctx, repo)
}

// send sends one request with the token of repo and decodes a JSON answer into out when out
// is not nil. The token never appears in an error.
func (c *Client) send(ctx context.Context, repo, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("github: %w", err)
		}
		body = bytes.NewReader(b)
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(base, "/")+path, body)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	token, err := c.token(ctx, repo)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("github: read %s: %w", path, err)
	}
	if len(data) > maxBody {
		return fmt.Errorf("github: %s: the answer is over %d bytes", path, maxBody)
	}
	if resp.StatusCode/100 != 2 {
		return &Error{Status: resp.StatusCode, Path: path, Body: snippet(data), RateLimited: rateLimited(resp.Header, data)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("github: decode %s: %w", path, err)
	}
	return nil
}

// Error is a non-2xx answer.
type Error struct {
	Status int
	Path   string
	Body   string
	// RateLimited marks a refusal for asking too much, which GitHub also sends as 403.
	RateLimited bool
}

// rateLimited reads the three ways GitHub says that a limit, primary or secondary, is hit.
func rateLimited(h http.Header, body []byte) bool {
	return h.Get("Retry-After") != "" || h.Get("X-RateLimit-Remaining") == "0" ||
		bytes.Contains(bytes.ToLower(body), []byte("rate limit"))
}

func (e *Error) Error() string {
	return fmt.Sprintf("github: %s: status %d: %s", e.Path, e.Status, e.Body)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

func repoPath(repo string) (string, error) {
	if !repoRe.MatchString(repo) {
		return "", fmt.Errorf("github: bad repository %q", repo)
	}
	return "/repos/" + repo, nil
}

type pullDoc struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	Merged  bool   `json:"merged"`
	HTMLURL string `json:"html_url"`
	NodeID  string `json:"node_id"`
	Head    struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

// OpenPR opens a pull request from a pushed branch.
func (c *Client) OpenPR(ctx context.Context, pr integration.OpenPR) (integration.PRRef, error) {
	base, err := repoPath(pr.Repo)
	if err != nil {
		return integration.PRRef{}, err
	}
	var d pullDoc
	err = c.do(ctx, http.MethodPost, base+"/pulls", map[string]any{
		"title": pr.Title, "head": pr.Head, "base": pr.Base, "body": pr.Body, "draft": pr.Draft,
	}, &d)
	if err != nil {
		return integration.PRRef{}, err
	}
	return integration.PRRef{Repo: pr.Repo, Number: d.Number, HeadSHA: d.Head.SHA}, nil
}

func (c *Client) pull(ctx context.Context, ref integration.PRRef) (pullDoc, error) {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return pullDoc{}, err
	}
	var d pullDoc
	err = c.do(ctx, http.MethodGet, base+"/pulls/"+strconv.Itoa(ref.Number), nil, &d)
	return d, err
}

// PRState reads the pull request.
func (c *Client) PRState(ctx context.Context, ref integration.PRRef) (integration.PRState, error) {
	d, err := c.pull(ctx, ref)
	if err != nil {
		return integration.PRState{}, err
	}
	state := d.State
	if d.Merged {
		state = "merged"
	}
	ref.HeadSHA = d.Head.SHA
	return integration.PRState{Ref: ref, State: state, Draft: d.Draft, URL: d.HTMLURL, Merged: d.Merged}, nil
}

// Merge squashes the pull request at ref.HeadSHA; GitHub refuses (409) when the head moved.
func (c *Client) Merge(ctx context.Context, ref integration.PRRef) (string, error) {
	var out struct {
		SHA    string `json:"sha"`
		Merged bool   `json:"merged"`
	}
	in := map[string]string{"sha": ref.HeadSHA, "merge_method": "squash"}
	if err := c.do(ctx, http.MethodPut, "/repos/"+ref.Repo+"/pulls/"+strconv.Itoa(ref.Number)+"/merge", in, &out); err != nil {
		return "", err
	}
	if !out.Merged {
		return "", fmt.Errorf("github: pull request %s#%d was not merged", ref.Repo, ref.Number)
	}
	return out.SHA, nil
}

// MarkReady takes the pull request out of draft. Only GraphQL can do that.
func (c *Client) MarkReady(ctx context.Context, ref integration.PRRef) error {
	d, err := c.pull(ctx, ref)
	if err != nil {
		return err
	}
	if !d.Draft {
		return nil
	}
	q := `mutation($id: ID!) { markPullRequestReadyForReview(input: {pullRequestId: $id}) { pullRequest { isDraft } } }`
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.send(ctx, ref.Repo, http.MethodPost, "/graphql", map[string]any{"query": q, "variables": map[string]string{"id": d.NodeID}}, &out); err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("github: mark ready: %s", out.Errors[0].Message)
	}
	return nil
}

// RequestReviewers asks people for a review.
func (c *Client) RequestReviewers(ctx context.Context, ref integration.PRRef, users []string) error {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, base+"/pulls/"+strconv.Itoa(ref.Number)+"/requested_reviewers", map[string]any{"reviewers": users}, nil)
}

// AddLabels adds labels to the pull request.
func (c *Client) AddLabels(ctx context.Context, ref integration.PRRef, labels []string) error {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return err
	}
	if len(labels) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, base+"/issues/"+strconv.Itoa(ref.Number)+"/labels", map[string]any{"labels": labels}, nil)
}

// SetStatus publishes a commit status, for example nightshift/human-review.
func (c *Client) SetStatus(ctx context.Context, repo, sha string, st integration.CommitStatus) error {
	base, err := repoPath(repo)
	if err != nil {
		return err
	}
	if !shaRe.MatchString(sha) {
		return fmt.Errorf("github: bad sha %q", sha)
	}
	switch st.State {
	case "pending", "success", "failure":
	default:
		return fmt.Errorf("github: bad status state %q", st.State)
	}
	return c.do(ctx, http.MethodPost, base+"/statuses/"+sha, map[string]any{
		"state": st.State, "context": st.Context, "description": truncate(st.Description, 140), "target_url": st.URL,
	}, nil)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// run is a check run or a workflow run; each endpoint fills its own fields.
type run struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	// A check run names its suite.
	CheckSuite struct {
		ID int64 `json:"id"`
	} `json:"check_suite"`
	// A workflow run names its workflow, its suite and where its code came from.
	WorkflowID     int64    `json:"workflow_id"`
	Event          string   `json:"event"`
	CheckSuiteID   int64    `json:"check_suite_id"`
	Repository     *repoRef `json:"repository"`
	HeadRepository *repoRef `json:"head_repository"`
}

type repoRef struct {
	ID int64 `json:"id"`
}

// foreign reports a workflow run whose code is not the repository's own. Anyone may fork a
// public repository and open a pull request from one of its commits, and that run reports on
// the same commit. A deleted fork leaves no head repository.
func (r run) foreign() bool {
	return r.Repository == nil || r.HeadRepository == nil || r.HeadRepository.ID != r.Repository.ID
}

func failed(conclusion string) bool {
	switch conclusion {
	case "failure", "cancelled", "timed_out", "action_required", "startup_failure": //nolint:misspell // the GitHub API spells the conclusion this way
		return true
	}
	return false
}

// runs lists what reported on a commit, without the Actions runs of forks. GitHub offers no checks
// permission to a fine-grained token, so on 403 it reads the Actions workflow runs of the
// commit instead; checks of other apps are then visible only if they also set a commit status.
func (c *Client) runs(ctx context.Context, base, sha string) ([]run, error) {
	checks, err := c.allRuns(ctx, base+"/commits/"+sha+"/check-runs?", "check_runs")
	if forbidden(err) {
		return c.latestWorkflowRuns(ctx, base, sha)
	}
	if err != nil || len(checks) == 0 {
		return checks, err
	}
	// Without this list the origin of a check is unknown, so any refusal, a rate limit
	// included, leaves the commit without a verdict.
	workflows, err := c.allRuns(ctx, base+"/actions/runs?head_sha="+sha+"&", "workflow_runs")
	if err != nil {
		return nil, err
	}
	foreign := map[int64]bool{}
	for _, w := range workflows {
		if w.foreign() {
			foreign[w.CheckSuiteID] = true
		}
	}
	return slices.DeleteFunc(checks, func(r run) bool { return foreign[r.CheckSuite.ID] }), nil
}

func (c *Client) latestWorkflowRuns(ctx context.Context, base, sha string) ([]run, error) {
	workflows, err := c.allRuns(ctx, base+"/actions/runs?head_sha="+sha+"&", "workflow_runs")
	if err != nil {
		return nil, err
	}
	// Newest first: an older run of the same workflow and event is superseded.
	seen := map[string]bool{}
	var latest []run
	for _, r := range workflows {
		key := strconv.FormatInt(r.WorkflowID, 10) + "/" + r.Event
		if !r.foreign() && !seen[key] {
			seen[key] = true
			latest = append(latest, r)
		}
	}
	return latest, nil
}

// allRuns reads every page of a run list; field names the list in the answer.
func (c *Client) allRuns(ctx context.Context, path, field string) ([]run, error) {
	var out []run
	for page := 1; page <= maxRunPages; page++ {
		var doc map[string]json.RawMessage
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%sper_page=%d&page=%d", path, runPage, page), nil, &doc); err != nil {
			return nil, err
		}
		var runs []run
		if err := json.Unmarshal(doc[field], &runs); err != nil {
			return nil, fmt.Errorf("github: %s: %w", field, err)
		}
		out = append(out, runs...)
		if len(runs) < runPage {
			return out, nil
		}
	}
	// A failed run could be past the last page, so a verdict on this list would be wrong.
	return nil, fmt.Errorf("github: more than %d %s on one commit", runPage*maxRunPages, field)
}

// forbidden reports a refusal of the credential itself, not of the pace of its requests.
func forbidden(err error) bool {
	var refused *Error
	return errors.As(err, &refused) && refused.Status == http.StatusForbidden && !refused.RateLimited
}

// Workflows counts the CI workflows of a repository; it fails when the credential cannot
// reach it, for example because the app is not installed there.
func (c *Client) Workflows(ctx context.Context, repo string) (int, error) {
	base, err := repoPath(repo)
	if err != nil {
		return 0, err
	}
	var out struct {
		Total int `json:"total_count"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/actions/workflows?per_page=1", nil, &out); err != nil {
		return 0, err
	}
	return out.Total, nil
}

// CI summarizes check runs and commit statuses of a commit.
func (c *Client) CI(ctx context.Context, repo, sha string) (integration.CIResult, error) {
	base, err := repoPath(repo)
	if err != nil {
		return integration.CIResult{}, err
	}
	if !shaRe.MatchString(sha) {
		return integration.CIResult{}, fmt.Errorf("github: bad sha %q", sha)
	}
	runs, err := c.runs(ctx, base, sha)
	if err != nil {
		return integration.CIResult{}, err
	}
	var combined struct {
		Statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
		} `json:"statuses"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/commits/"+sha+"/status?per_page=100", nil, &combined); err != nil {
		return integration.CIResult{}, err
	}
	res := integration.CIResult{State: integration.CISuccess}
	reported, pending := len(runs), false
	for _, r := range runs {
		switch {
		case r.Status != "completed":
			pending = true
		case failed(r.Conclusion):
			res.Failing = append(res.Failing, r.Name)
		}
	}
	for _, s := range combined.Statuses {
		switch {
		case strings.HasPrefix(s.Context, ownStatusPrefix):
			continue
		case s.State == "pending":
			pending = true
		case s.State == "failure" || s.State == "error":
			res.Failing = append(res.Failing, s.Context)
		}
		reported++
	}
	pending = pending || reported == 0 // nothing but the factory itself has reported
	switch {
	case len(res.Failing) > 0:
		res.State = integration.CIFailure
	case pending:
		res.State = integration.CIPending
	}
	return res, nil
}

// The page limits bound what one pull request can make the factory read.
const (
	runPage         = 100
	maxRunPages     = 10
	reviewPage      = 100 // the code host's largest page
	maxReviewPages  = 50
	maxCommentPages = 2
)

// Reviews lists the reviews of the pull request, oldest first.
func (c *Client) Reviews(ctx context.Context, ref integration.PRRef) ([]integration.Review, error) {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return nil, err
	}
	out := []integration.Review{}
	for page := 1; page <= maxReviewPages; page++ {
		var docs []struct {
			ID   int64 `json:"id"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
			State    string `json:"state"`
			CommitID string `json:"commit_id"`
			Body     string `json:"body"`
		}
		path := fmt.Sprintf("%s/pulls/%d/reviews?per_page=%d&page=%d", base, ref.Number, reviewPage, page)
		if err := c.do(ctx, http.MethodGet, path, nil, &docs); err != nil {
			return nil, err
		}
		for _, d := range docs {
			out = append(out, integration.Review{ID: d.ID, User: d.User.Login, State: strings.ToLower(d.State), CommitSHA: d.CommitID, Body: d.Body})
		}
		if len(docs) < reviewPage {
			return out, nil
		}
	}
	// A newer review could be past the last page, so deciding on this list would be wrong.
	return nil, fmt.Errorf("github: pull request %d has more than %d reviews", ref.Number, reviewPage*maxReviewPages)
}

// LineComments renders the comments a review left on lines of the diff: the file, the last
// line of the hunk a comment points at, and the text.
func (c *Client) LineComments(ctx context.Context, ref integration.PRRef, review int64) (string, error) {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for page := 1; page <= maxCommentPages; page++ {
		var docs []struct {
			Path     string `json:"path"`
			DiffHunk string `json:"diff_hunk"`
			Body     string `json:"body"`
		}
		path := fmt.Sprintf("%s/pulls/%d/reviews/%d/comments?per_page=%d&page=%d", base, ref.Number, review, reviewPage, page)
		if err := c.do(ctx, http.MethodGet, path, nil, &docs); err != nil {
			return "", err
		}
		for _, d := range docs {
			b.WriteString("\n" + d.Path)
			if hunk := strings.TrimRight(d.DiffHunk, "\n"); hunk != "" {
				fmt.Fprintf(&b, " at `%s`", hunk[strings.LastIndex(hunk, "\n")+1:])
			}
			b.WriteString(": " + d.Body)
		}
		if len(docs) < reviewPage {
			return b.String(), nil
		}
	}
	b.WriteString("\n(more comments on the pull request were left out)")
	return b.String(), nil
}
