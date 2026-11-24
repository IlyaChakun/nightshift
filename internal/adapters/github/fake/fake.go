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
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Check is a scripted check run.
type Check struct {
	Name, Status, Conclusion string
}

// Review is a scripted review.
type Review struct {
	User, State, CommitSHA string
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

	mu       sync.Mutex
	srv      *httptest.Server
	prs      map[string]*PR // "repo#n"
	next     int
	checks   map[string][]Check // "repo@sha"
	statuses map[string][]status
	reviews  map[string][]Review // "repo#n"
}

// New starts a twin that wants the bearer token.
func New(t *testing.T, token string) *Server {
	t.Helper()
	s := &Server{Token: token, prs: map[string]*PR{}, checks: map[string][]Check{}, statuses: map[string][]status{}, reviews: map[string][]Review{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls", s.open)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{n}", s.get)
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls/{n}/requested_reviewers", s.reviewers)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{n}/reviews", s.listReviews)
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues/{n}/labels", s.labels)
	mux.HandleFunc("POST /repos/{owner}/{repo}/statuses/{sha}", s.setStatus)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/check-runs", s.checkRuns)
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{sha}/status", s.combined)
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

func prDoc(p *PR) map[string]any {
	return map[string]any{
		"number": p.Number, "state": p.State, "draft": p.Draft, "merged": p.Merged,
		"html_url": fmt.Sprintf("https://github.test/%s/pull/%d", p.Repo, p.Number),
		"node_id":  "PR_" + key(p.Repo, p.Number), "head": map[string]string{"sha": p.HeadSHA},
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
	s.next++
	repo := r.PathValue("owner") + "/" + r.PathValue("repo")
	p := &PR{Repo: repo, Number: s.next, Title: in.Title, Head: in.Head, Base: in.Base, Draft: in.Draft, State: "open", HeadSHA: sha(in.Head + "0")}
	s.prs[key(repo, p.Number)] = p
	reply(w, http.StatusCreated, prDoc(p))
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pr(r)
	if !ok {
		reply(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	reply(w, http.StatusOK, prDoc(p))
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
	reply(w, http.StatusCreated, prDoc(p))
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
	runs := []map[string]string{}
	for _, c := range s.checks[r.PathValue("owner")+"/"+r.PathValue("repo")+"@"+r.PathValue("sha")] {
		runs = append(runs, map[string]string{"name": c.Name, "status": c.Status, "conclusion": c.Conclusion})
	}
	reply(w, http.StatusOK, map[string]any{"total_count": len(runs), "check_runs": runs})
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
	for _, rv := range s.reviews[r.PathValue("owner")+"/"+r.PathValue("repo")+"#"+r.PathValue("n")] {
		out = append(out, map[string]any{"user": map[string]string{"login": rv.User}, "state": strings.ToUpper(rv.State), "commit_id": rv.CommitSHA})
	}
	reply(w, http.StatusOK, out)
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
	s.checks[repo+"@"+commit] = checks
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
