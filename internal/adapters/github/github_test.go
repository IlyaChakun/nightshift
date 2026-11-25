package github_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/integration"
)

const (
	token = "ghs-test-token-never-in-errors" //nolint:gosec // G101: a fixed fake token for tests
	repo  = "example/app"
)

func setup(t *testing.T) (*github.Client, *fake.Server) {
	t.Helper()
	srv := fake.New(t, token)
	return &github.Client{BaseURL: srv.URL, Token: token}, srv
}

func TestHandOverFlow(t *testing.T) {
	t.Parallel()
	c, srv := setup(t)
	ctx := t.Context()
	ref, err := c.OpenPR(ctx, integration.OpenPR{Repo: repo, Head: "ns/1", Base: "main", Title: "feat: csv export", Body: "what and why", Draft: true})
	if err != nil || ref.Number != 1 || len(ref.HeadSHA) != 40 {
		t.Fatalf("open: %+v, %v", ref, err)
	}
	if st, err := c.PRState(ctx, ref); err != nil || st.State != "open" || !st.Draft {
		t.Fatalf("state: %+v, %v", st, err)
	}
	if err := c.MarkReady(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkReady(ctx, ref); err != nil { // already ready: nothing to do
		t.Fatal(err)
	}
	if err := c.RequestReviewers(ctx, ref, []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if err := c.AddLabels(ctx, ref, []string{"needs-human-review"}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetStatus(ctx, repo, ref.HeadSHA, integration.CommitStatus{Context: "nightshift/human-review", State: "pending", Description: "waiting for the owner"}); err != nil {
		t.Fatal(err)
	}
	got, _ := srv.PR(repo, 1)
	if got.Draft || !slices.Equal(got.Reviewers, []string{"alice"}) || !slices.Equal(got.Labels, []string{"needs-human-review"}) {
		t.Fatalf("pull request: %+v", got)
	}
	if srv.Status(repo, ref.HeadSHA, "nightshift/human-review") != "pending" {
		t.Error("the status was not published")
	}

	srv.AddReview(repo, 1, fake.Review{
		User: "alice", State: "changes_requested", CommitSHA: ref.HeadSHA, Body: "Please add a test.",
		Comments: []fake.LineComment{{Path: "a.go", Body: "This panics on nil."}},
	})
	if rv, err := c.Reviews(ctx, ref); err != nil || len(rv) != 1 || rv[0].State != "changes_requested" || rv[0].User != "alice" || rv[0].Body != "Please add a test." {
		t.Fatalf("reviews: %+v, %v", rv, err)
	}
	srv.Merge(repo, 1)
	if st, err := c.PRState(ctx, ref); err != nil || st.State != "merged" || !st.Merged {
		t.Fatalf("after the team merged: %+v, %v", st, err)
	}
}

func TestReviewsAndLineCommentsArePaged(t *testing.T) {
	t.Parallel()
	c, srv := setup(t)
	ctx := t.Context()
	ref := integration.PRRef{Repo: repo, Number: 1}
	comments := []fake.LineComment{{Path: "a.go", DiffHunk: "@@ -1 +1,2 @@\n x\n+y := *p", Body: "This panics on nil."}}
	for range 40 {
		comments = append(comments, fake.LineComment{Path: "b.go", Body: "And here."})
	}
	srv.AddReview(repo, 1, fake.Review{User: "alice", State: "changes_requested", Comments: comments})
	lines, err := c.LineComments(ctx, ref, 1)
	if err != nil || !strings.HasPrefix(lines, "\na.go at `+y := *p`: This panics on nil.") || strings.Count(lines, "b.go") != 40 {
		t.Fatalf("line comments: %q, %v", lines, err)
	}
	for range 40 { // a second page: the newest review must still be seen
		srv.AddReview(repo, 1, fake.Review{User: "stranger", State: "commented"})
	}
	srv.AddReview(repo, 1, fake.Review{User: "alice", State: "approved"})
	if rv, err := c.Reviews(ctx, ref); err != nil || len(rv) != 42 || rv[41].State != "approved" {
		t.Fatalf("paged reviews: %d, %v", len(rv), err)
	}
}

func TestCISummary(t *testing.T) {
	t.Parallel()
	c, srv := setup(t)
	ctx := t.Context()
	head := strings.Repeat("a", 40)
	ci := func() integration.CIResult {
		t.Helper()
		r, err := c.CI(ctx, repo, head)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := ci(); r.State != integration.CIPending {
		t.Errorf("nothing reported yet: %+v", r)
	}
	srv.SetChecks(repo, head, fake.Check{Name: "lint", Status: "completed", Conclusion: "success"}, fake.Check{Name: "test", Status: "in_progress"})
	if r := ci(); r.State != integration.CIPending {
		t.Errorf("a check still runs: %+v", r)
	}
	srv.SetChecks(repo, head, fake.Check{Name: "lint", Status: "completed", Conclusion: "success"}, fake.Check{Name: "test", Status: "completed", Conclusion: "failure"})
	if r := ci(); r.State != integration.CIFailure || !slices.Equal(r.Failing, []string{"test"}) {
		t.Errorf("a check failed: %+v", r)
	}
	srv.SetChecks(repo, head, fake.Check{Name: "lint", Status: "completed", Conclusion: "success"})
	srv.SetStatus(repo, head, "nightshift/human-review", "pending") // the factory does not wait for itself
	if r := ci(); r.State != integration.CISuccess {
		t.Errorf("all green: %+v", r)
	}
	srv.SetStatus(repo, head, "external/sonar", "failure")
	if r := ci(); r.State != integration.CIFailure || !slices.Equal(r.Failing, []string{"external/sonar"}) {
		t.Errorf("an external status failed: %+v", r)
	}
}

// A fine-grained token gets 403 on check runs, as on the first live run.
func TestCIFromWorkflowRunsWhenChecksAreForbidden(t *testing.T) {
	t.Parallel()
	c, srv := setup(t)
	srv.ForbidChecks()
	head := strings.Repeat("b", 40)
	want := func(state integration.CIState, failing ...string) {
		t.Helper()
		r, err := c.CI(t.Context(), repo, head)
		if err != nil || r.State != state || !slices.Equal(r.Failing, failing) {
			t.Errorf("want %v %v, got %+v, %v", state, failing, r, err)
		}
	}
	want(integration.CIPending)
	srv.SetChecks(repo, head, fake.Check{Name: "ci", Status: "in_progress"})
	want(integration.CIPending)
	// The newer run of a workflow comes first and supersedes the failed one.
	srv.SetChecks(repo, head, fake.Check{Name: "ci", Status: "completed", Conclusion: "success"}, fake.Check{Name: "ci", Status: "completed", Conclusion: "failure"})
	want(integration.CISuccess)
	srv.SetChecks(repo, head, fake.Check{Name: "ci", Status: "completed", Conclusion: "failure"})
	want(integration.CIFailure, "ci")
}

func TestErrorsNeverCarryTheToken(t *testing.T) {
	t.Parallel()
	srv := fake.New(t, token)
	bad := &github.Client{BaseURL: srv.URL, Token: "wrong-secret-token"}
	_, err := bad.OpenPR(t.Context(), integration.OpenPR{Repo: repo, Head: "ns/1", Base: "main"})
	var ge *github.Error
	if !errors.As(err, &ge) || ge.Status != 401 || strings.Contains(err.Error(), "wrong-secret-token") {
		t.Fatalf("error: %v", err)
	}
}

func TestBadInputIsRefusedBeforeTheNetwork(t *testing.T) {
	t.Parallel()
	c := &github.Client{BaseURL: "http://127.0.0.1:1", Token: token} // nothing listens: any request would fail differently
	ctx := t.Context()
	for name, err := range map[string]error{
		"repo with a path":   func() error { _, err := c.OpenPR(ctx, integration.OpenPR{Repo: "a/b/../../c"}); return err }(),
		"repo without owner": func() error { _, err := c.CI(ctx, "app", strings.Repeat("a", 40)); return err }(),
		"short sha":          func() error { _, err := c.CI(ctx, repo, "abc"); return err }(),
		"status state":       c.SetStatus(ctx, repo, strings.Repeat("a", 40), integration.CommitStatus{Context: "x", State: "great"}),
	} {
		if err == nil || !strings.Contains(err.Error(), "bad") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMergeRefusesAMovedHead(t *testing.T) {
	t.Parallel()
	c, srv := setup(t)
	ctx := t.Context()
	ref, err := c.OpenPR(ctx, integration.OpenPR{Repo: repo, Head: "ns/2", Base: "main", Title: "feat: x"})
	if err != nil {
		t.Fatal(err)
	}
	stale := ref
	srv.PushHead(repo, ref.Number, "new")
	if _, err := c.Merge(ctx, stale); err == nil {
		t.Fatal("merged a head that moved")
	}
	ref.HeadSHA = srv.PushHead(repo, ref.Number, "newer")
	if sha, err := c.Merge(ctx, ref); err != nil || len(sha) != 40 {
		t.Fatalf("merge: %q %v", sha, err)
	}
	if st, _ := c.PRState(ctx, ref); !st.Merged {
		t.Fatalf("state: %+v", st)
	}
}
