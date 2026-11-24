// Package github is the code-host adapter: the REST calls the factory needs to hand work to
// people (design sections 7 and 11). It uses the standard library only. Authentication is a
// token read from the secrets directory by the caller; GitHub App tokens come with the real
// infrastructure check.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

// maxBody caps what is read from a response, so a broken server cannot flood memory.
const maxBody = 4 << 20

// ownStatusPrefix marks the factory's own statuses; CI ignores them.
const ownStatusPrefix = "nightshift/"

// Client talks to the code host.
type Client struct {
	BaseURL string // defaults to DefaultBaseURL
	Token   string
	HTTP    *http.Client // defaults to a client with a 30 second timeout
}

var _ integration.PullRequests = (*Client)(nil)

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// do sends one request and decodes a JSON answer into out when out is not nil. The token
// never appears in an error.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
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
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("github: read %s: %w", path, err)
	}
	if resp.StatusCode/100 != 2 {
		return &Error{Status: resp.StatusCode, Path: path, Body: snippet(data)}
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
	if err := c.do(ctx, http.MethodPost, "/graphql", map[string]any{"query": q, "variables": map[string]string{"id": d.NodeID}}, &out); err != nil {
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

// CI summarizes check runs and commit statuses of a commit.
func (c *Client) CI(ctx context.Context, repo, sha string) (integration.CIResult, error) {
	base, err := repoPath(repo)
	if err != nil {
		return integration.CIResult{}, err
	}
	if !shaRe.MatchString(sha) {
		return integration.CIResult{}, fmt.Errorf("github: bad sha %q", sha)
	}
	var runs struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/commits/"+sha+"/check-runs?per_page=100", nil, &runs); err != nil {
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
	pending := len(runs.CheckRuns) == 0 && len(combined.Statuses) == 0 // nothing has reported yet
	for _, r := range runs.CheckRuns {
		switch {
		case r.Status != "completed":
			pending = true
		case r.Conclusion == "failure" || r.Conclusion == "cancelled" || r.Conclusion == "timed_out" || r.Conclusion == "action_required": //nolint:misspell // the GitHub API spells the conclusion this way
			res.Failing = append(res.Failing, r.Name)
		}
	}
	for _, s := range combined.Statuses {
		switch {
		case strings.HasPrefix(s.Context, ownStatusPrefix):
		case s.State == "pending":
			pending = true
		case s.State == "failure" || s.State == "error":
			res.Failing = append(res.Failing, s.Context)
		}
	}
	switch {
	case len(res.Failing) > 0:
		res.State = integration.CIFailure
	case pending:
		res.State = integration.CIPending
	}
	return res, nil
}

// Reviews lists the reviews of the pull request, oldest first.
func (c *Client) Reviews(ctx context.Context, ref integration.PRRef) ([]integration.Review, error) {
	base, err := repoPath(ref.Repo)
	if err != nil {
		return nil, err
	}
	var docs []struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		State    string `json:"state"`
		CommitID string `json:"commit_id"`
	}
	if err := c.do(ctx, http.MethodGet, base+"/pulls/"+strconv.Itoa(ref.Number)+"/reviews?per_page=100", nil, &docs); err != nil {
		return nil, err
	}
	out := make([]integration.Review, 0, len(docs))
	for _, d := range docs {
		out = append(out, integration.Review{User: d.User.Login, State: strings.ToLower(d.State), CommitSHA: d.CommitID})
	}
	return out, nil
}
