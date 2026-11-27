package main

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/customization/config"
)

func TestReplyArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []string
		id   int64
		text string
		ok   bool
	}{
		{"words are joined", []string{"12", "All", "columns"}, 12, "All columns", true},
		{"a hash prefix is allowed", []string{"#7", "yes"}, 7, "yes", true},
		{"no text", []string{"12"}, 0, "", false},
		{"blank text", []string{"12", "  "}, 0, "", false},
		{"not a number", []string{"abc", "yes"}, 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, text, ok := replyArgs(tt.in)
			if ok != tt.ok || (ok && (id != tt.id || text != tt.text)) {
				t.Fatalf("got %d %q %v", id, text, ok)
			}
		})
	}
}

func TestPickTenant(t *testing.T) {
	t.Parallel()
	one := &config.Instance{Name: "i", Tenants: map[string]config.Tenant{"a": {Name: "a"}}}
	two := &config.Instance{Name: "i", Tenants: map[string]config.Tenant{"a": {Name: "a"}, "b": {Name: "b"}}}
	if got, err := pickTenant(one, ""); err != nil || got.Name != "a" {
		t.Fatalf("the only tenant: %v %v", got, err)
	}
	if _, err := pickTenant(two, ""); err == nil || !strings.Contains(err.Error(), "a, b") {
		t.Fatalf("two tenants need a flag: %v", err)
	}
	if _, err := pickTenant(two, "c"); err == nil {
		t.Fatal("an unknown tenant was accepted")
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	if got := renderStatus(nil); got != "no open tasks\n" {
		t.Fatal(got)
	}
	if got := renderStatus([]doors.TaskLine{{ID: 3, Title: "Add CSV export", State: "grilling"}}); !strings.Contains(got, "#3  grilling") {
		t.Fatal(got)
	}
	if got := renderInbox(nil); got != "nothing waits for you\n" {
		t.Fatal(got)
	}
	question := renderInbox([]doors.Pending{{TaskID: 3, Title: "T", Text: "Which columns?", Kind: chatapi.CardQuestion}})
	spec := renderInbox([]doors.Pending{{TaskID: 4, Title: "T", Version: 6, Text: "Spec", Buttons: []chatapi.Button{{Action: "approve"}, {Action: "reject"}}}})
	if !strings.Contains(question, `nightshift reply 3`) || !strings.Contains(spec, "buttons: approve, reject (nightshift approve 4 6)") {
		t.Fatalf("question:\n%s\nspec:\n%s", question, spec)
	}
}

// Regression: `status --task 0` printed the open tasks instead of saying the id is wrong.
func TestStatusTaskNeedsAnID(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"0", "-4"} {
		var out, errb strings.Builder
		if code := doorCmd("status", []string{"--task", arg}, &out, &errb); code != exitUsage || !strings.Contains(errb.String(), "--task needs a task id") {
			t.Errorf("--task %s: code %d, stderr %q", arg, code, errb.String())
		}
	}
}

func TestApproveArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		id   int64
		card int
		ok   bool
	}{
		{[]string{"12"}, 12, 0, true},
		{[]string{"#12", "3"}, 12, 3, true},
		{[]string{"12", "0"}, 0, 0, false},
		{[]string{"12", "x"}, 0, 0, false},
		{[]string{"12", "3", "4"}, 0, 0, false},
		{nil, 0, 0, false},
	} {
		id, card, ok := approveArgs(tc.args)
		if ok != tc.ok || (ok && (id != tc.id || card != tc.card)) {
			t.Errorf("approveArgs(%q) = %d, %d, %v; want %d, %d, %v", tc.args, id, card, ok, tc.id, tc.card, tc.ok)
		}
	}
}
