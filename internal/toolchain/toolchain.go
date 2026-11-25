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

// Request asks for the install of the toolchain pinned in Dir.
type Request struct {
	Job string `json:"job"`
	Dir string `json:"dir"`
}

// Response carries the tail of the install output.
type Response struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// Server runs `mise install` for a workspace. It accepts one request per job, and only for
// paths under Root, so a caller that gets past the token cannot install from an arbitrary place.
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
	seen map[string]bool
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
	s.mu.Lock()
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	again := s.seen[req.Job]
	s.seen[req.Job] = true
	s.mu.Unlock()
	if again {
		http.Error(w, "this job has been installed already", http.StatusConflict)
		return
	}
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
	_ = json.NewEncoder(w).Encode(resp)
}

// within returns dir cleaned, when it is a directory under Root.
func (s *Server) within(dir string) (string, error) {
	dir = filepath.Clean(dir)
	rel, err := filepath.Rel(s.Root, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("the directory is outside the work root")
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", errors.New("the directory does not exist")
	}
	return dir, nil
}

// mise installs the pinned tools as the agent user, with the service's own proxy settings.
func (s *Server) mise(ctx context.Context, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "mise", "install", "--yes")
	var out bytes.Buffer
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, &out, &out
	cmd.WaitDelay = time.Second
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // mise needs the service's PATH
		"HOME=" + s.AgentHome, "MISE_YES=1", "MISE_TRUSTED_CONFIG_PATHS=" + dir,
		"HTTPS_PROXY=" + os.Getenv("HTTPS_PROXY"), //nolint:forbidigo // the install proxy is the service's own way out
		"NO_PROXY=" + os.Getenv("NO_PROXY"),       //nolint:forbidigo // same
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

// Install blocks until the service has installed the tools in dir. The output tail comes back
// with the error, so the job fails with what mise said.
func (c Client) Install(ctx context.Context, job, dir string) (string, error) {
	body, err := json.Marshal(Request{Job: job, Dir: dir})
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
