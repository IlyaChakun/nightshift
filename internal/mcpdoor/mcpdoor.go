// Package mcpdoor serves the inbox doors as MCP tools (ADR 0006), on the official Go SDK.
// The tools are thin wrappers over the same calls as the terminal commands, so every door
// follows one logic path. Over stdio the client starts the process and no port opens.
package mcpdoor

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

// Backend is what the tools call: one tenant's doors, already bound to the door's source.
type Backend interface {
	Status(ctx context.Context) ([]dispatch.TaskLine, error)
	Inbox(ctx context.Context) ([]dispatch.Pending, error)
	AddIdea(ctx context.Context, repo, text string) (id int64, title string, err error)
	Reply(ctx context.Context, task int64, text string) error
	Approve(ctx context.Context, task int64) error
}

// Tool names.
const (
	ToolTaskAdd = "task_add"
	ToolStatus  = "status"
	ToolInbox   = "inbox"
	ToolReply   = "reply"
	ToolApprove = "approve"
)

// Options tune a server. Allowed limits the tools it offers; nil offers all of them.
type Options struct {
	Version string
	Allowed map[string]bool
}

type taskAddIn struct {
	Text string `json:"text" jsonschema:"the idea, in a sentence or two"`
	Repo string `json:"repo,omitempty" jsonschema:"owner/name; default the tenant's first repository"`
}

type taskAddOut struct {
	Task  int64  `json:"task"`
	Title string `json:"title"`
}

type noIn struct{}

type statusOut struct {
	Tasks []dispatch.TaskLine `json:"tasks"`
}

type inboxOut struct {
	Cards []dispatch.Pending `json:"cards"`
}

type replyIn struct {
	Task int64  `json:"task" jsonschema:"the task number"`
	Text string `json:"text" jsonschema:"the answer to the last question, or a note for Lead"`
}

type approveIn struct {
	Task int64 `json:"task" jsonschema:"the task number whose spec card is waiting"`
}

type okOut struct {
	OK bool `json:"ok"`
}

// New builds the server with the tools of the doors.
func New(b Backend, o Options) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "nightshift", Version: o.Version}, nil)
	on := func(name string) bool { return o.Allowed == nil || o.Allowed[name] }
	if on(ToolTaskAdd) {
		mcp.AddTool(s, &mcp.Tool{Name: ToolTaskAdd, Description: "Start a task from an idea. Lead answers with a question or a spec; read it with inbox."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in taskAddIn) (*mcp.CallToolResult, taskAddOut, error) {
				id, title, err := b.AddIdea(ctx, in.Repo, in.Text)
				return nil, taskAddOut{Task: id, Title: title}, err
			})
	}
	if on(ToolStatus) {
		mcp.AddTool(s, &mcp.Tool{Name: ToolStatus, Description: "List the open tasks and their states."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, statusOut, error) {
				tasks, err := b.Status(ctx)
				return nil, statusOut{Tasks: orEmpty(tasks)}, err
			})
	}
	if on(ToolInbox) {
		mcp.AddTool(s, &mcp.Tool{Name: ToolInbox, Description: "Show the cards that wait for the owner's answer: Lead's questions and spec cards."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, inboxOut, error) {
				cards, err := b.Inbox(ctx)
				return nil, inboxOut{Cards: orEmpty(cards)}, err
			})
	}
	if on(ToolReply) {
		mcp.AddTool(s, &mcp.Tool{Name: ToolReply, Description: "Answer Lead's question in a task thread. Say what the owner said, in their words."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in replyIn) (*mcp.CallToolResult, okOut, error) {
				err := b.Reply(ctx, in.Task, in.Text)
				return nil, okOut{OK: err == nil}, err
			})
	}
	if on(ToolApprove) {
		mcp.AddTool(s, &mcp.Tool{Name: ToolApprove, Description: "Approve the spec card of a task so Builder starts. Only when the owner said so."},
			func(ctx context.Context, _ *mcp.CallToolRequest, in approveIn) (*mcp.CallToolResult, okOut, error) {
				err := b.Approve(ctx, in.Task)
				return nil, okOut{OK: err == nil}, err
			})
	}
	return s
}

// Serve runs the server over stdio until the client closes it or ctx ends.
func Serve(ctx context.Context, s *mcp.Server) error {
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp: %w", err)
	}
	return nil
}

// orEmpty keeps a list in the JSON result as [] instead of null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Doors binds one tenant's dispatch.Doors to a door source ("mcp", "mcp-remote") as a Backend.
type Doors struct {
	Doors  *dispatch.Doors
	Tenant config.Tenant
	Source string
}

var _ Backend = Doors{}

// Status implements Backend.
func (d Doors) Status(ctx context.Context) ([]dispatch.TaskLine, error) {
	return d.Doors.Status(ctx, d.Tenant)
}

// Inbox implements Backend.
func (d Doors) Inbox(ctx context.Context) ([]dispatch.Pending, error) {
	return d.Doors.Inbox(ctx, d.Tenant)
}

// AddIdea implements Backend.
func (d Doors) AddIdea(ctx context.Context, repo, text string) (int64, string, error) {
	task, err := d.Doors.AddIdea(ctx, d.Tenant, repo, text, d.Source)
	return task.ID, task.Title, err
}

// Reply implements Backend.
func (d Doors) Reply(ctx context.Context, task int64, text string) error {
	return d.Doors.Reply(ctx, d.Tenant, task, text, d.Source)
}

// Approve implements Backend.
func (d Doors) Approve(ctx context.Context, task int64) error {
	return d.Doors.Approve(ctx, d.Tenant, task, d.Source)
}
