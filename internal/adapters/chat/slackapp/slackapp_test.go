package slackapp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
)

func TestPost(t *testing.T) {
	t.Parallel()
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		form = r.PostForm
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"200.1"}`))
	}))
	defer srv.Close()
	a := New(Options{BotToken: "test-bot", AppToken: "test-app", APIURL: srv.URL + "/"})
	ref, err := a.Post(t.Context(), chatapi.Card{
		TaskID: "42", Version: 3, Kind: chatapi.CardDecisionNeeded, Text: "Spec ready",
		Buttons: []chatapi.Button{{Action: "approve", Label: "Approve"}},
		Thread:  chatapi.MessageRef{Channel: "C1", ID: "100.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref != (chatapi.MessageRef{Channel: "C1", ID: "100.1"}) {
		t.Errorf("ref = %+v, want the task thread", ref)
	}
	for key, want := range map[string]string{"channel": "C1", "thread_ts": "100.1", "unfurl_links": "false", "unfurl_media": "false"} {
		if got := form.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if b := form.Get("blocks"); !strings.Contains(b, `"action_id":"approve"`) || !strings.Contains(b, `"value":"42@3"`) {
		t.Errorf("blocks = %s", b)
	}
}

func TestPostStartsThread(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"300.1"}`))
	}))
	defer srv.Close()
	a := New(Options{APIURL: srv.URL + "/"})
	ref, err := a.Post(t.Context(), chatapi.Card{Text: "hi", Thread: chatapi.MessageRef{Channel: "C1"}})
	if err != nil || ref.ID != "300.1" {
		t.Fatalf("ref %+v, %v", ref, err)
	}
	if _, err := a.Post(t.Context(), chatapi.Card{Text: "no channel"}); err == nil {
		t.Error("card without channel accepted")
	}
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	msg := func(m slackevents.MessageEvent) socketmode.Event {
		return socketmode.Event{Type: socketmode.EventTypeEventsAPI, Data: slackevents.EventsAPIEvent{
			InnerEvent: slackevents.EventsAPIInnerEvent{Data: &m},
		}}
	}
	click := func(value string) socketmode.Event {
		var cb slack.InteractionCallback
		cb.Type = slack.InteractionTypeBlockActions
		cb.User.ID = "U0OWNER"
		cb.Channel.ID = "C1"
		cb.Message.Timestamp = "100.1"
		cb.ActionCallback.BlockActions = []*slack.BlockAction{{ActionID: "approve", Value: value}}
		return socketmode.Event{Type: socketmode.EventTypeInteractive, Data: cb}
	}
	tests := []struct {
		name string
		ev   socketmode.Event
		want *chatapi.ChatEvent
	}{
		{"message in a thread", msg(slackevents.MessageEvent{User: "U0OWNER", Text: "Add CSV export", Channel: "C1", TimeStamp: "101.1", ThreadTimeStamp: "100.1"}),
			&chatapi.ChatEvent{Kind: chatapi.ChatMessage, User: "U0OWNER", Text: "Add CSV export", MessageID: "101.1", Thread: chatapi.MessageRef{Channel: "C1", ID: "100.1"}, At: now}},
		{"top-level message starts a thread", msg(slackevents.MessageEvent{User: "U0OWNER", Text: "idea", Channel: "C1", TimeStamp: "102.1"}),
			&chatapi.ChatEvent{Kind: chatapi.ChatMessage, User: "U0OWNER", Text: "idea", MessageID: "102.1", Thread: chatapi.MessageRef{Channel: "C1", ID: "102.1"}, At: now}},
		{"client attribution is dropped", msg(slackevents.MessageEvent{User: "U0OWNER", Text: "idea *Sent using* <@U1|Client>", Channel: "C1", TimeStamp: "103.1"}),
			&chatapi.ChatEvent{Kind: chatapi.ChatMessage, User: "U0OWNER", Text: "idea", MessageID: "103.1", Thread: chatapi.MessageRef{Channel: "C1", ID: "103.1"}, At: now}},
		{"the words in a sentence stay", msg(slackevents.MessageEvent{User: "U0OWNER", Text: "Sent using the API, it fails", Channel: "C1", TimeStamp: "104.1"}),
			&chatapi.ChatEvent{Kind: chatapi.ChatMessage, User: "U0OWNER", Text: "Sent using the API, it fails", MessageID: "104.1", Thread: chatapi.MessageRef{Channel: "C1", ID: "104.1"}, At: now}},
		{"own message", msg(slackevents.MessageEvent{User: "UBOT", Text: "x", Channel: "C1"}), nil},
		{"edit", msg(slackevents.MessageEvent{User: "U0OWNER", SubType: "message_changed", Channel: "C1"}), nil},
		{"button", click("42@3"),
			&chatapi.ChatEvent{Kind: chatapi.ChatButton, User: "U0OWNER", Action: "approve", TaskID: "42", CardVersion: 3, Thread: chatapi.MessageRef{Channel: "C1", ID: "100.1"}, At: now}},
		{"forged button value", click("42"), nil},
		{"slash command", socketmode.Event{Type: socketmode.EventTypeSlashCommand, Data: slack.SlashCommand{UserID: "U0OWNER", ChannelID: "C1", Text: " cancel 42 "}},
			&chatapi.ChatEvent{Kind: chatapi.ChatCommand, User: "U0OWNER", Text: "cancel 42", Thread: chatapi.MessageRef{Channel: "C1"}, At: now}},
		{"hello", socketmode.Event{Type: socketmode.EventTypeHello}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := translate(tc.ev, "UBOT", now)
			if tc.want == nil {
				if ok {
					t.Fatalf("got %+v, want it ignored", got)
				}
				return
			}
			if !ok || got != *tc.want {
				g, _ := json.Marshal(got)
				w, _ := json.Marshal(tc.want)
				t.Fatalf("got %s, want %s", g, w)
			}
		})
	}
}

func FuzzParseButtonValue(f *testing.F) {
	for _, s := range []string{"42@3", "@1", "42@0", "42@x", "", "a@b@c"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, v string) {
		task, version, err := parseButtonValue(v)
		if err != nil {
			return
		}
		if task == "" || version < 1 || !strings.HasPrefix(v, task+"@") {
			t.Fatalf("parse(%q) = %q, %d", v, task, version)
		}
	})
}

// TestEventsEndWhenSocketGivesUp checks that a connection that cannot recover, here a
// revoked app token, closes the stream instead of leaving the caller waiting forever.
func TestEventsEndWhenSocketGivesUp(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth.test"):
			_, _ = w.Write([]byte(`{"ok":true,"user_id":"UBOT"}`))
		default: // apps.connections.open
			_, _ = w.Write([]byte(`{"ok":false,"error":"token_revoked"}`))
		}
	}))
	defer srv.Close()
	a := New(Options{BotToken: "test-bot", AppToken: "test-app", APIURL: srv.URL + "/", Log: slog.New(slog.DiscardHandler)})
	events, err := a.Events(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case _, open := <-events:
			if !open {
				return
			}
		case <-timeout:
			t.Fatal("stream still open after the socket gave up")
		}
	}
}

func TestReact(t *testing.T) {
	t.Parallel()
	var form url.Values
	reply := `{"ok":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		form = r.PostForm
		_, _ = w.Write([]byte(reply))
	}))
	defer srv.Close()
	a := New(Options{BotToken: "test-bot", AppToken: "test-app", APIURL: srv.URL + "/"})
	msg := chatapi.MessageRef{Channel: "C1", ID: "101.1"}
	if err := a.React(t.Context(), msg, "eyes"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"channel": "C1", "timestamp": "101.1", "name": "eyes"} {
		if got := form.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	reply = `{"ok":false,"error":"already_reacted"}`
	if err := a.React(t.Context(), msg, "eyes"); err != nil {
		t.Errorf("a repeated reaction must not fail: %v", err)
	}
	reply = `{"ok":false,"error":"missing_scope"}`
	if err := a.React(t.Context(), msg, "eyes"); err == nil {
		t.Error("a missing scope must be reported")
	}
}

func TestLongCardTextIsSplitUnderSlacksLimit(t *testing.T) {
	text := strings.Repeat("a line of a spec\n", 400)
	parts := splitText(text, maxSectionRunes)
	if len(parts) < 2 || strings.Join(parts, "") != text {
		t.Fatalf("parts: %d, lossless: %v", len(parts), strings.Join(parts, "") == text)
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > 3000 {
			t.Fatalf("a part has %d runes", n)
		}
	}
	if got := splitText("short", maxSectionRunes); len(got) != 1 || got[0] != "short" {
		t.Fatalf("short text: %v", got)
	}
}

func TestRulesInATextBecomeDividers(t *testing.T) {
	got := blocks(chatapi.Card{Text: "*Head*\nsummary\n---\n*Needs you*\n• one\n---\n*Merged*"})
	var kinds []string
	for _, b := range got {
		kinds = append(kinds, string(b.BlockType()))
	}
	if want := "section divider section divider section"; strings.Join(kinds, " ") != want {
		t.Fatalf("blocks: %v", kinds)
	}
	if one := blocks(chatapi.Card{Text: "no rule here"}); len(one) != 1 {
		t.Fatalf("a text without a rule is one section: %d", len(one))
	}
	if none := blocks(chatapi.Card{Text: "a\n---\n"}); len(none) != 1 {
		t.Fatalf("a trailing rule adds no empty section: %d", len(none))
	}
}
