package claude_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/claude"
)

// argAfter returns the value of a flag in args.
func argAfter(args []string, flag string) string {
	if i := slices.Index(args, flag); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// An acceptance turn browses through the browser tool and writes nothing else: it runs only the
// listed tools (dontAsk), the shell and the file tools are refused, and so are the browser tools
// that run code or read local files (owner decision 2026-11-28), and no subagent is started.
func TestAnAcceptanceTurnGetsOnlyTheBrowserTool(t *testing.T) {
	req := request(t)
	req.Role.ReadOnly = true
	req.Browser = &harness.Browser{EvidenceDir: "/jobs/1/evidence", Config: "/jobs/1/browser.json"}
	h := &claude.Harness{Auth: claude.Subscription, PermissionMode: "acceptEdits",
		AllowedTools: []string{"Read", "Edit", "Write", "Bash", "Agent"}, BrowserMCP: "/opt/playwright-mcp/bin/playwright-mcp"}
	args := h.Args(req)
	if argAfter(args, "--permission-mode") != "dontAsk" || !slices.Contains(args, "--strict-mcp-config") {
		t.Fatalf("args: %v", args)
	}
	allowed := strings.Split(argAfter(args, "--allowedTools"), ",")
	denied := strings.Split(argAfter(args, "--disallowedTools"), ",")
	// Agent: the acceptance job of V4 on the Mac spawned a subagent (2026-11-28).
	for _, tool := range []string{"Bash", "Write", "Edit", "NotebookEdit", "Agent", "Task"} {
		if slices.Contains(allowed, tool) || !slices.Contains(denied, tool) {
			t.Errorf("%s: allowed %v, denied %v", tool, allowed, denied)
		}
	}
	for _, tool := range []string{"browser_run_code_unsafe", "browser_evaluate", "browser_file_upload", "browser_drop"} {
		if !slices.Contains(denied, "mcp__browser__"+tool) || slices.Contains(allowed, "mcp__browser__"+tool) {
			t.Errorf("%s is not refused: %v", tool, denied)
		}
	}
	for _, tool := range []string{"Read", "Glob", "Grep", "mcp__browser__browser_navigate", "mcp__browser__browser_take_screenshot"} {
		if !slices.Contains(allowed, tool) {
			t.Errorf("%s is not allowed: %v", tool, allowed)
		}
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(argAfter(args, "--mcp-config")), &cfg); err != nil {
		t.Fatal(err)
	}
	srv := cfg.MCPServers["browser"]
	// The server starts in the evidence directory and ignores the client's roots: the worktree is
	// not one of the places it may write.
	if len(cfg.MCPServers) != 1 || srv.Command != "/bin/sh" || !slices.Contains(srv.Args, "/jobs/1/evidence") ||
		!strings.Contains(strings.Join(srv.Args, " "), "--ignore-roots") || !strings.Contains(strings.Join(srv.Args, " "), "umask 022") || slices.Contains(srv.Args, req.Workspace) {
		t.Errorf("browser server: %+v", cfg)
	}
}

// A turn without a browser keeps the role's tools and gets no MCP server.
func TestAReviewTurnGetsNoBrowser(t *testing.T) {
	req := request(t)
	req.Role.ReadOnly = true
	h := &claude.Harness{Auth: claude.Subscription, AllowedTools: []string{"Read", "Bash"}, BrowserMCP: "/opt/playwright-mcp/bin/playwright-mcp"}
	args := h.Args(req)
	if slices.Contains(args, "--mcp-config") || argAfter(args, "--allowedTools") != "Read,Bash" {
		t.Errorf("args: %v", args)
	}
}

// A worker without the browser tool refuses an acceptance turn instead of running it blind.
func TestAnAcceptanceTurnNeedsTheBrowserTool(t *testing.T) {
	req := request(t)
	req.Browser = &harness.Browser{EvidenceDir: t.TempDir(), Config: "x.json"}
	if _, err := (&claude.Harness{Bin: "/nonexistent"}).Run(t.Context(), req); err == nil || !strings.Contains(err.Error(), "BrowserMCP") {
		t.Fatalf("err = %v", err)
	}
	// A path the image did not install (an empty PLAYWRIGHT_MCP_VERSION) is refused too.
	missing := filepath.Join(t.TempDir(), "playwright-mcp")
	if _, err := (&claude.Harness{Bin: "/nonexistent", BrowserMCP: missing}).Run(t.Context(), req); err == nil || !strings.Contains(err.Error(), "BrowserMCP") {
		t.Fatalf("missing server: err = %v", err)
	}
}
