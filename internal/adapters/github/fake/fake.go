// Package fake is a twin of the code host for tests: an HTTP server that speaks the REST
// subset the github adapter uses, keeps pull requests in memory, and lets a test script CI,
// reviews, merges and new heads.
package fake

import (
	"crypto/sha1" //nolint:gosec // G505: fake commit ids, not security
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Check is a scripted check run.
type Check struct {
	Name, Status, Conclusion string
	Fork                     bool // run for a pull request from a fork
	Deleted                  bool // that fork is gone
	// Summary is the output of the run; Annotations are its messages ("path:line: text").
	Summary     string
	Annotations []string
	id          int64
}

// Review is a scripted review.
type Review struct {
	User, State, CommitSHA string
	Body                   string
	Comments               []LineComment
}

// LineComment is a comment of a review on a file of the diff.
type LineComment struct {
	Path, DiffHunk, Body string
}

// PR is a pull request of the twin.
type PR struct {
	Repo      string
	Number    int
	Title     string
	Head      string // branch
	Base      string
	HeadSHA   string
	Draft     bool
	State     string // open or closed
	Merged    bool
	Labels    []string
	Reviewers []string
}

type status struct{ Context, State, Description string }

// Server is the twin. Create it with New and read its URL.
type Server struct {
	URL   string
	Token string

	// HeadOf, when set, tells the twin where a branch really points, as GitHub knows it from
	// the pushed commits. Without it the head is the one the twin made up.
	HeadOf func(repo, branch string) string

	mu           sync.Mutex
	srv          *httptest.Server
	prs          map[string]*PR // "repo#n"
	next         int
	checks       map[string][]Check // "repo@sha"
	lastCheckID  int64
	statuses     map[string][]status
	reviews      map[string][]Review // "repo#n"
	noChecks     bool
	noActions    bool
	checkLimit   *Refusal
	noComments   map[int]bool // review id; 0 fails every review
	commentCalls int
}

// New starts a twin that wants the bearer token.
func New(t *testing.T, token string) *Server {
	t.Helper()
	s := &Server{Token: token, prs: map[string]*PR{}, checks: map[string][]Check{}, statuses: map[string][]status{}, reviews: map[string][]Review{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls", s.open)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls", s.list)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{n}", s.get)
	mux.HandleFunc("PUT /repos/{owner}/{repo}/pulls/{n}/merge", s.merge)
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls/{n}/requested_reviewers", s.reviewers)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{n}/reviews", s.listReviews)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{n}/reviews/{id}/comments", s.reviewComments)
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues/{n}/labels", s.labels)
	mux.HandleFunc("POST /repos/{owner}/{repo}/statuses/{sha}", s.setStatus)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/check-runs", s.checkRuns)
	mux.HandleFunc("GET /repos/{owner}/{repo}/check-runs/{id}/annotations", s.annotations)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/status", s.combined)
	mux.HandleFunc("GET /repos/{owner}/{repo}/actions/runs", s.workflowRuns)
	mux.HandleFunc("POST /graphql", s.graphql)
	s.srv = httptest.NewServer(s.auth(mux))
	t.Cleanup(s.srv.Close)
	s.URL = s.srv.URL
	return s
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.Token {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func key(repo string, n int) string { return repo + "#" + strconv.Itoa(n) }

func sha(seed string) string {
	h := sha1.Sum([]byte(seed)) //nolint:gosec // G401: fake commit ids
	return hex.EncodeToString(h[:])
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) pr(r *http.Request) (*PR, bool) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		return nil, false
	}
	p, ok := s.prs[key(r.PathValue("owner")+"/"+r.PathValue("repo"), n)]
	return p, ok
}

// head is the commit the pull request points at; the caller holds the lock.
func (s *Server) head(p *PR) string {
	if s.HeadOf != nil {
		if h := s.HeadOf(p.Repo, p.Head); h != "" {
			return h
		}
	}
	return p.HeadSHA
}

func (s *Server) prDoc(p *PR) map[string]any {
	return map[string]any{
		"number": p.Number, "state": p.State, "draft": p.Draft, "merged": p.Merged,
		"html_url": fmt.Sprintf("https://github.test/%s/pull/%d", p.Repo, p.Number),
		"node_id":  "PR_" + key(p.Repo, p.Number), "head": map[string]string{"sha": s.head(p)},
	}
}

func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title, Head, Base string
		Draft             bool
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Head == "" || in.Base == "" {
		reply(w, http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := r.PathValue("owner") + "/" + r.PathValue("repo")
	for _, p := range s.prs {
		if p.Repo == repo && p.Head == in.Head && p.State == "open" { // GitHub refuses a second open pull request for a branch
			reply(w, http.StatusUnprocessableEntity, map[string]any{"message": "Validation Failed", "errors": []map[string]string{{"message": "A pull request already exists for " + r.PathValue("owner") + ":" + in.Head + "."}}})
			return
		}
	}
	s.next++
	p := &PR{Repo: repo, Number: s.next, Title: in.Title, Head: in.Head, Base: in.Base, Draft: in.Draft, State: "open", HeadSHA: sha(in.Head + "0")}
	s.prs[key(repo, p.Number)] = p
	reply(w, http.StatusCreated, s.prDoc(p))
}

// list answers the open pull requests of a repository, filtered by `head` ("owner:branch") and
// `base`, as far as the factory asks.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := r.PathValue("owner") + "/" + r.PathValue("repo")
	q := r.URL.Query()
	out := []map[string]any{}
	for _, p := range s.prs {
		if p.Repo != repo || p.State != "open" {
			continue
		}
		if h := q.Get("head"); h != "" && h != r.PathValue("owner")+":"+p.Head {
			continue
		}
		if b := q.Get("base"); b != "" && b != p.Base {
			continue
		}
		out = append(out, s.prDoc(p))
	}
	reply(w, http.StatusOK, out)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pr(r)
	if !ok {
		reply(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	reply(w, http.StatusOK, s.prDoc(p))
}

func (s *Server) merge(w http.ResponseWriter, r *http.Request) {
	var in struct{ SHA string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		reply(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pr(r)
	switch {
	case !ok:
		reply(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	case p.Merged || in.SHA != s.head(p):
		reply(w, http.StatusConflict, map[string]string{"message": "Head branch was modified. Review and try the merge again."})
	default:
		p.Merged, p.State = true, "closed"
		reply(w, http.StatusOK, map[string]any{"sha": sha(p.Head + "merge"), "merged": true})
	}
}

func (s *Server) reviewers(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reviewers []string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pr(r)
	if !ok {
		reply(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	p.Reviewers = append(p.Reviewers, in.Reviewers...)
	reply(w, http.StatusCreated, s.prDoc(p))
}

func (s *Server) labels(w http.ResponseWriter, r *http.Request) {
	var in struct{ Labels []string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pr(r)
	if !ok {
		reply(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	p.Labels = append(p.Labels, in.Labels...)
	reply(w, http.StatusOK, in.Labels)
}

func (s *Server) setStatus(w http.ResponseWriter, r *http.Request) {
	var in struct{ State, Context, Description string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	k := r.PathValue("owner") + "/" + r.PathValue("repo") + "@" + r.PathValue("sha")
	list := s.statuses[k]
	for i := range list { // a context keeps only its latest state, like the real host
		if list[i].Context == in.Context {
			list[i] = status{in.Context, in.State, in.Description}
			reply(w, http.StatusCreated, in)
			return
		}
	}
	s.statuses[k] = append(list, status{in.Context, in.State, in.Description})
	reply(w, http.StatusCreated, in)
}

func (s *Server) checkRuns(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checkLimit != nil {
		for k, v := range s.checkLimit.Header {
			w.Header().Set(k, v)
		}
		reply(w, http.StatusForbidden, map[string]string{"message": s.checkLimit.Message})
		return
	}
	if s.noChecks {
		reply(w, http.StatusForbidden, map[string]string{"message": "Resource not accessible by personal access token"})
		return
	}
	runs := []map[string]any{}
	for i, c := range s.checks[r.PathValue("owner")+"/"+r.PathValue("repo")+"@"+r.PathValue("sha")] {
		runs = append(runs, map[string]any{
			"id": c.id, "name": c.Name, "status": c.Status, "conclusion": c.Conclusion, "check_suite": map[string]int{"id": i + 1},
			"output": map[string]string{"title": c.Name + " failed", "summary": c.Summary},
		})
	}
	reply(w, http.StatusOK, map[string]any{"total_count": len(runs), "check_runs": pageOf(r, runs)})
}

// ForbidChecks makes the twin answer as GitHub does to a fine-grained token: no check runs,
// only workflow runs, which then carry the scripted checks.
func (s *Server) ForbidChecks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noChecks = true
}

// Refusal is a scripted 403 answer.
type Refusal struct {
	Message string
	Header  map[string]string
}

// LimitChecks refuses the list of check runs the way a rate limit does.
func (s *Server) LimitChecks(r Refusal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkLimit = &r
}

// ForbidActions refuses the list of workflow runs, as a rate limit or a missing permission
// does.
func (s *Server) ForbidActions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noActions = true
}

// workflowRuns lists the scripted checks newest first; a name is one workflow.
func (s *Server) workflowRuns(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.noActions {
		reply(w, http.StatusForbidden, map[string]string{"message": "API rate limit exceeded"})
		return
	}
	runs := []map[string]any{}
	ids := map[string]int{}
	for i, c := range s.checks[r.PathValue("owner")+"/"+r.PathValue("repo")+"@"+r.URL.Query().Get("head_sha")] {
		if _, ok := ids[c.Name]; !ok {
			ids[c.Name] = len(ids) + 1
		}
		head := 1
		if c.Fork {
			head = 2
		}
		doc := map[string]any{
			"name": c.Name, "status": c.Status, "conclusion": c.Conclusion, "workflow_id": ids[c.Name], "event": "pull_request",
			"check_suite_id": i + 1, "repository": map[string]int{"id": 1}, "head_repository": map[string]int{"id": head},
		}
		if c.Deleted {
			doc["head_repository"] = nil
		}
		runs = append(runs, doc)
	}
	reply(w, http.StatusOK, map[string]any{"workflow_runs": pageOf(r, runs)})
}

func (s *Server) combined(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []map[string]string{}
	for _, st := range s.statuses[r.PathValue("owner")+"/"+r.PathValue("repo")+"@"+r.PathValue("sha")] {
		list = append(list, map[string]string{"context": st.Context, "state": st.State})
	}
	reply(w, http.StatusOK, map[string]any{"statuses": list})
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for i, rv := range s.reviews[r.PathValue("owner")+"/"+r.PathValue("repo")+"#"+r.PathValue("n")] {
		out = append(out, map[string]any{
			"id": i + 1, "user": map[string]string{"login": rv.User}, "state": strings.ToUpper(rv.State),
			"commit_id": rv.CommitSHA, "body": rv.Body,
		})
	}
	reply(w, http.StatusOK, pageOf(r, out))
}

// pageOf pages a list as the code host does: per_page items of the 1-based page.
func pageOf[T any](r *http.Request, list []T) []T {
	size, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if size <= 0 || page <= 0 {
		return list
	}
	from := min((page-1)*size, len(list))
	return list[from:min(from+size, len(list))]
}

// FailLineComments makes the requests for the line comments of the reviews with these ids
// fail; the id 0 stands for every review. Without ids the requests work again.
func (s *Server) FailLineComments(ids ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noComments = map[int]bool{}
	for _, id := range ids {
		s.noComments[id] = true
	}
}

// LineCommentCalls counts the requests for line comments.
func (s *Server) LineCommentCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commentCalls
}

// reviewComments lists the line comments of one review; its id is its position, from 1.
func (s *Server) reviewComments(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reviews := s.reviews[r.PathValue("owner")+"/"+r.PathValue("repo")+"#"+r.PathValue("n")]
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 || id > len(reviews) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	s.commentCalls++
	if s.noComments[0] || s.noComments[id] {
		http.Error(w, `{"message":"Server Error"}`, http.StatusInternalServerError)
		return
	}
	out := []map[string]string{}
	for _, c := range reviews[id-1].Comments {
		out = append(out, map[string]string{"path": c.Path, "diff_hunk": c.DiffHunk, "body": c.Body})
	}
	reply(w, http.StatusOK, pageOf(r, out))
}

// graphql understands only markPullRequestReadyForReview.
func (s *Server) graphql(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string
		Variables map[string]string
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strings.TrimPrefix(in.Variables["id"], "PR_")
	p, ok := s.prs[id]
	if !strings.Contains(in.Query, "markPullRequestReadyForReview") || !ok {
		reply(w, http.StatusOK, map[string]any{"errors": []map[string]string{{"message": "unsupported or unknown pull request"}}})
		return
	}
	p.Draft = false
	reply(w, http.StatusOK, map[string]any{"data": map[string]any{}})
}

// PR returns a copy of a pull request, for assertions.
func (s *Server) PR(repo string, n int) (PR, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.prs[key(repo, n)]
	if !ok {
		return PR{}, false
	}
	return *p, true
}

// Status returns the state of a status context on a commit ("" when none).
func (s *Server) Status(repo, commit, context string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.statuses[repo+"@"+commit] {
		if st.Context == context {
			return st.State
		}
	}
	return ""
}

// SetChecks scripts the check runs of a commit.
func (s *Server) SetChecks(repo, commit string, checks ...Check) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A copy: a test may hand the same slice to several servers.
	checks = slices.Clone(checks)
	for i := range checks {
		s.lastCheckID++
		checks[i].id = s.lastCheckID
	}
	s.checks[repo+"@"+commit] = checks
}

// annotations answers the messages of one check run, by the id the twin gave it.
func (s *Server) annotations(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for _, list := range s.checks {
		for _, c := range list {
			if strconv.FormatInt(c.id, 10) != r.PathValue("id") {
				continue
			}
			for _, a := range c.Annotations {
				out = append(out, map[string]any{"path": "x.go", "start_line": 1, "message": a, "annotation_level": "failure"})
			}
		}
	}
	reply(w, http.StatusOK, out)
}

// SetStatus scripts a commit status from another system, such as an external CI.
func (s *Server) SetStatus(repo, commit, context, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[repo+"@"+commit] = append(s.statuses[repo+"@"+commit], status{Context: context, State: state})
}

// AddReview scripts a review on a pull request.
func (s *Server) AddReview(repo string, n int, rv Review) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews[key(repo, n)] = append(s.reviews[key(repo, n)], rv)
}

// Merge marks a pull request merged, as the team would do in the `team` profile.
func (s *Server) Merge(repo string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.prs[key(repo, n)]; ok {
		p.Merged, p.State = true, "closed"
	}
}

// Close closes a pull request without merging.
func (s *Server) Close(repo string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.prs[key(repo, n)]; ok {
		p.State = "closed"
	}
}

// PushHead gives a pull request a new head commit, as a new push to its branch would.
func (s *Server) PushHead(repo string, n int, seed string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.prs[key(repo, n)]
	if !ok {
		return ""
	}
	p.HeadSHA = sha(p.Head + seed)
	return p.HeadSHA
}
