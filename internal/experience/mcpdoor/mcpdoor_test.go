package mcpdoor_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/experience/mcpdoor"
)

type backend struct {
	replies  []string
	approved []int64
	cards    []int
	err      error
}

func (b *backend) Status(context.Context) ([]doors.TaskLine, error) {
	return []doors.TaskLine{{ID: 3, Title: "Add CSV export", State: "grilling"}}, nil
}

func (b *backend) Inbox(context.Context) ([]doors.Pending, error) {
	return []doors.Pending{{TaskID: 3, Title: "Add CSV export", Version: 2, Kind: chatapi.CardQuestion, Text: "Which columns?"}}, nil
}

func (b *backend) AddIdea(_ context.Context, _, text string) (int64, string, error) {
	return 9, text, b.err
}

func (b *backend) Reply(_ context.Context, _ int64, text string) error {
	b.replies = append(b.replies, text)
	return b.err
}

func (b *backend) Approve(_ context.Context, task int64, card int) error {
	b.approved = append(b.approved, task)
	b.cards = append(b.cards, card)
	return b.err
}

// connect plays a client over an in-memory transport.
func connect(t *testing.T, b mcpdoor.Backend, o mcpdoor.Options) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	server, client := mcp.NewInMemoryTransports()
	srv := mcpdoor.New(b, o)
	ss, err := srv.Connect(ctx, server, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, client, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
}

func TestToolsAreThinWrappersOverTheBackend(t *testing.T) {
	t.Parallel()
	b := &backend{}
	cs := connect(t, b, mcpdoor.Options{})

	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "approve,inbox,reply,status,task_add" {
		t.Fatalf("tools: %v", names)
	}

	res, err := call(t, cs, mcpdoor.ToolInbox, map[string]any{})
	if err != nil || res.IsError {
		t.Fatalf("inbox: %v %+v", err, res)
	}
	var inbox struct{ Cards []struct{ Text string } }
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &inbox); err != nil || len(inbox.Cards) != 1 || inbox.Cards[0].Text != "Which columns?" {
		t.Fatalf("inbox result: %s %v", raw, err)
	}

	if res, err := call(t, cs, mcpdoor.ToolReply, map[string]any{"task": 3, "text": "All columns"}); err != nil || res.IsError {
		t.Fatalf("reply: %v %+v", err, res)
	}
	if res, err := call(t, cs, mcpdoor.ToolApprove, map[string]any{"task": 3}); err == nil && !res.IsError {
		t.Fatal("an approval that names no card was accepted")
	}
	if res, err := call(t, cs, mcpdoor.ToolApprove, map[string]any{"task": 3, "card": 5}); err != nil || res.IsError {
		t.Fatalf("approve: %v %+v", err, res)
	}
	if res, err := call(t, cs, mcpdoor.ToolTaskAdd, map[string]any{"text": "Add CSV export"}); err != nil || res.IsError {
		t.Fatalf("task_add: %v %+v", err, res)
	}
	if res, err := call(t, cs, mcpdoor.ToolStatus, map[string]any{}); err != nil || res.IsError {
		t.Fatalf("status: %v %+v", err, res)
	}
	if len(b.replies) != 1 || b.replies[0] != "All columns" || len(b.approved) != 1 || b.approved[0] != 3 || b.cards[0] != 5 {
		t.Fatalf("backend saw replies %v approved %v on cards %v", b.replies, b.approved, b.cards)
	}
}

func TestABackendErrorIsAToolError(t *testing.T) {
	t.Parallel()
	cs := connect(t, &backend{err: errors.New("task 3 waits for nothing")}, mcpdoor.Options{})
	res, err := call(t, cs, mcpdoor.ToolApprove, map[string]any{"task": 3, "card": 1})
	if err != nil {
		t.Fatalf("a refused approval must reach the client as a tool result, got a protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("result: %+v", res)
	}
}

func TestAllowedLimitsTheTools(t *testing.T) {
	t.Parallel()
	cs := connect(t, &backend{}, mcpdoor.Options{Allowed: map[string]bool{mcpdoor.ToolStatus: true}})
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != mcpdoor.ToolStatus {
		t.Fatalf("tools: %+v %v", tools, err)
	}
	if _, err := call(t, cs, mcpdoor.ToolApprove, map[string]any{"task": 3}); err == nil {
		t.Fatal("a tool that is not offered was called")
	}
}
