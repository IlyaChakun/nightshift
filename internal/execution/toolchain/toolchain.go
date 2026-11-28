// Package toolchain installs the tools a project pins, in a service of its own, so the agent's
// container needs no route to the tool hosts (design section 16, ADR 0010).
package toolchain

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// maxBody bounds a request; it carries a job id and a path.
const maxBody = 4 << 10

// outTail is how much of the install output goes back to the runner.
const outTail = 4 << 10

// Request asks for the install of the toolchain pinned in Dir. Attempt names one lease of the
// job (runner.attemptOf): a job that lost its worker comes back with a new one.
type Request struct {
	Job     string `json:"job"`
	Attempt string `json:"attempt,omitempty"`
	Dir     string `json:"dir"`
}

// Response carries the tail of the install output.
type Response struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Server runs `mise install` for a workspace, only for paths under Root, so a caller that gets
// past the token cannot install from an arbitrary place. It accepts one request per attempt of a
// job: a retried job installs again in its fresh checkout, but never while another attempt of
// the job installs, and an attempt it has seen, the current or an older one, is refused.
type Server struct {
	Token string
	// Root is the directory that holds the task checkouts.
	Root string
	// AgentHome and the ids describe the user the tools are installed for.
	AgentHome          string
	AgentUID, AgentGID int
	// Run installs the toolchain in dir; the default runs mise as the agent user.
	Run func(ctx context.Context, dir string) ([]byte, error)

	mu   sync.Mutex
	jobs map[string]*jobState
}

// jobState is what the server remembers of a job: the attempts it has seen and whether one of
// them is installing now.
type jobState struct {
	attempts map[string]bool
	running  bool
}

// Handler serves POST /install.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /install", s.install)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	want := "Bearer " + s.Token
	if s.Token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req Request
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil || req.Job == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	dir, err := s.within(req.Dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Attempt != "" && !strings.HasSuffix(filepath.Base(dir), "-"+req.Attempt) {
		http.Error(w, "the directory is not the checkout of this attempt", http.StatusBadRequest)
		return
	}
	if msg := s.begin(req.Job, req.Attempt); msg != "" {
		http.Error(w, msg, http.StatusConflict)
		return
	}
	defer s.end(req.Job)
	run := s.Run
	if run == nil {
		run = s.mise
	}
	out, err := run(r.Context(), dir)
	resp := Response{Output: tail(out)}
	if err != nil {
		resp.Error = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp) //nolint:errchkjson // a plain struct always encodes; a write error means the client is gone
}

// begin records an attempt of a job and marks the job as installing. It returns why the attempt
// is refused: the attempt was seen before, or another attempt of the job is installing.
func (s *Server) begin(job, attempt string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[string]*jobState{}
	}
	st := s.jobs[job]
	if st == nil {
		st = &jobState{attempts: map[string]bool{}}
		s.jobs[job] = st
	}
	switch {
	case st.attempts[attempt]:
		return "this attempt of the job has been installed already"
	case st.running:
		return "another attempt of this job is installing"
	}
	st.attempts[attempt] = true
	st.running = true
	return ""
}

// end marks the job as no longer installing.
func (s *Server) end(job string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job].running = false
}

// within returns dir with its symlinks resolved, when it is a directory under Root. The check
// runs twice, on the cleaned path and on the resolved one, so a link inside Root cannot lead out.
func (s *Server) within(dir string) (string, error) {
	errOutside := errors.New("the directory is outside the work root")
	root := s.root()
	if root == "" || !filepath.IsAbs(dir) {
		return "", errOutside
	}
	prefix := root + string(filepath.Separator)
	dir = filepath.Clean(dir)
	// A client may use the configured spelling of the work root or its physical path.
	// The resolved directory must still be a child of the physical root below.
	if !strings.HasPrefix(dir, prefix) && !strings.HasPrefix(dir, filepath.Clean(s.Root)+string(filepath.Separator)) {
		return "", errOutside
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", errors.New("the directory does not exist")
	}
	if !strings.HasPrefix(dir, prefix) {
		return "", errOutside
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", errors.New("the directory does not exist")
	}
	return dir, nil
}

// root is Root, cleaned and with its symlinks resolved; empty when Root is not set.
func (s *Server) root() string {
	if s.Root == "" {
		return ""
	}
	root := filepath.Clean(s.Root)
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	return root
}

// homeOf is the HOME the tools of a checkout are installed under: one directory per tenant, the
// first segment of the checkout's path below Root, the same place the worker runs the tenant's jobs
// in (runner.HomeFor). Outside Root there is no tenant, so the plain agent home stays.
func (s *Server) homeOf(dir string) string {
	rel, err := filepath.Rel(s.root(), dir)
	if err != nil || s.Root == "" || s.AgentHome == "" {
		return s.AgentHome
	}
	tenant, _, _ := strings.Cut(rel, string(filepath.Separator))
	if tenant == "" || tenant == "." || tenant == ".." {
		return s.AgentHome
	}
	return filepath.Join(s.AgentHome, "tenants", tenant)
}

// mise installs the pinned tools as the agent user, with the service's own proxy settings.
func (s *Server) mise(ctx context.Context, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "mise", "install", "--yes") //nolint:gosec // fixed arguments; only the working directory varies, and it was checked to be under the work root
	var out bytes.Buffer
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, &out, &out
	cmd.WaitDelay = time.Second
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // mise needs the service's PATH
		"HOME=" + s.homeOf(dir), "MISE_YES=1", "MISE_TRUSTED_CONFIG_PATHS=" + dir,
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"), //nolint:forbidigo // the install proxy is the service's own way out
		"NO_PROXY=" + os.Getenv("NO_PROXY"),       //nolint:forbidigo // same
		"GOPROXY=" + os.Getenv("GOPROXY"),         //nolint:forbidigo // tools built by Go come from the module cache, as in the worker
	}
	if s.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(s.AgentUID), Gid: uint32(s.AgentGID)}} //nolint:gosec // ids come from the service's own configuration
	}
	err := cmd.Run()
	return out.Bytes(), err
}

func tail(b []byte) string {
	if len(b) > outTail {
		b = b[len(b)-outTail:]
	}
	return string(b)
}

// Client asks the service to install a toolchain.
type Client struct {
	URL   string
	Token string
	HTTP  *http.Client
}

// Install blocks until the service has installed the tools in dir for one attempt of a job.
// The output tail comes back with the error, so the job fails with what mise said.
func (c Client) Install(ctx context.Context, job, attempt, dir string) (string, error) {
	body, err := json.Marshal(Request{Job: job, Attempt: attempt, Dir: dir})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/install", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return "", fmt.Errorf("toolchain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("toolchain: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var r Response
	if err := json.NewDecoder(io.LimitReader(resp.Body, outTail*2)).Decode(&r); err != nil {
		return "", fmt.Errorf("toolchain: %w", err)
	}
	if r.Error != "" {
		return r.Output, errors.New(r.Error)
	}
	return r.Output, nil
}
