package mcpdoor

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Source names of the doors in the book.
const (
	SourceStdio  = "mcp"
	SourceRemote = "mcp-remote"
)

// RemoteHandler serves the tools over streamable HTTP for a claude.ai project, which cannot
// reach a local process. Every request needs `Authorization: Bearer <token>`. The caller picks
// the tools (Options.Allowed) and binds the listener to loopback; see ValidateListen.
func RemoteHandler(b Backend, token string, o Options) (http.Handler, error) {
	if len(token) < minTokenLen {
		return nil, fmt.Errorf("mcp remote: the token must be at least %d characters", minTokenLen)
	}
	want := sha256.Sum256([]byte(token))
	srv := New(b, o)
	inner := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nightshift"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}

// minTokenLen keeps a guessable token out; `openssl rand -hex 32` gives 64.
const minTokenLen = 32

// ValidateListen accepts only a loopback address: the person exposes the door with an outbound
// tunnel of their choice, so the host itself opens no port (ADR 0006).
func ValidateListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("mcp remote: listen %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("mcp remote: listen %q is not a loopback address; use 127.0.0.1 and a tunnel", addr)
	}
	return nil
}
