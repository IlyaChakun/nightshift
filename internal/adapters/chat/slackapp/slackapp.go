// Package slackapp is the chat adapter for the factory's own Slack app over Socket Mode:
// no public endpoint, no inbound ports (design sections 3 and 7).
package slackapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Adapter implements integration.Chat.
type Adapter struct {
	api    *slack.Client
	socket *socketmode.Client
	log    *slog.Logger
	botID  string
}

var _ integration.Chat = (*Adapter)(nil)

// Options configure the adapter; the tokens come from compose secrets.
type Options struct {
	BotToken string // xoxb-…
	AppToken string // xapp-…, for Socket Mode
	APIURL   string // tests point it at a local server
	Log      *slog.Logger
}

// New returns an adapter; nothing connects until Events is called.
func New(o Options) *Adapter {
	opts := []slack.Option{slack.OptionAppLevelToken(o.AppToken)}
	if o.APIURL != "" {
		opts = append(opts, slack.OptionAPIURL(o.APIURL))
	}
	api := slack.New(o.BotToken, opts...)
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{api: api, socket: socketmode.New(api), log: log}
}

// Post sends a card into its thread with link and media unfurling off, so a link in a
// message cannot leak data to a third-party preview fetcher (design section 16).
func (a *Adapter) Post(ctx context.Context, c integration.Card) (integration.MessageRef, error) {
	if c.Thread.Channel == "" {
		return integration.MessageRef{}, errors.New("slackapp: card has no channel")
	}
	opts := []slack.MsgOption{
		slack.MsgOptionText(c.Text, false),
		slack.MsgOptionBlocks(blocks(c)...),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	}
	if c.Thread.ID != "" {
		opts = append(opts, slack.MsgOptionTS(c.Thread.ID))
	}
	channel, ts, err := a.api.PostMessageContext(ctx, c.Thread.Channel, opts...)
	if err != nil {
		return integration.MessageRef{}, fmt.Errorf("slackapp: post: %w", err)
	}
	thread := c.Thread.ID
	if thread == "" {
		thread = ts // the first message starts the task's thread
	}
	return integration.MessageRef{Channel: channel, ID: thread}, nil
}

func blocks(c integration.Card) []slack.Block {
	out := []slack.Block{slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, c.Text, false, false), nil, nil)}
	if len(c.Buttons) == 0 {
		return out
	}
	elems := make([]slack.BlockElement, 0, len(c.Buttons))
	for _, b := range c.Buttons {
		btn := slack.NewButtonBlockElement(b.Action, buttonValue(c.TaskID, c.Version),
			slack.NewTextBlockObject(slack.PlainTextType, b.Label, false, false))
		elems = append(elems, btn)
	}
	return append(out, slack.NewActionBlock("card", elems...))
}

// buttonValue carries the task and the card version, so a click on a stale card is rejected.
func buttonValue(taskID string, version int) string {
	return taskID + "@" + strconv.Itoa(version)
}

func parseButtonValue(v string) (string, int, error) {
	task, ver, ok := strings.Cut(v, "@")
	if !ok || task == "" {
		return "", 0, fmt.Errorf("slackapp: bad button value %q", v)
	}
	n, err := strconv.Atoi(ver)
	if err != nil || n < 1 {
		return "", 0, fmt.Errorf("slackapp: bad card version in %q", v)
	}
	return task, n, nil
}

// Events connects over Socket Mode and translates what people do into chat events.
// Every request is acknowledged at once; work happens after the ack.
func (a *Adapter) Events(ctx context.Context) (<-chan integration.ChatEvent, error) {
	auth, err := a.api.AuthTestContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("slackapp: auth: %w", err)
	}
	a.botID = auth.UserID
	out := make(chan integration.ChatEvent)
	go func() {
		// RunContext returns only when the context ends or the connection gives up.
		if err := a.socket.RunContext(ctx); !errors.Is(err, context.Canceled) {
			a.log.Error("socket mode stopped", "error", err)
		}
	}()
	go a.pump(ctx, out)
	return out, nil
}

func (a *Adapter) pump(ctx context.Context, out chan<- integration.ChatEvent) {
	defer close(out)
	for {
		var ev socketmode.Event
		select {
		case <-ctx.Done():
			return
		case e, ok := <-a.socket.Events:
			if !ok {
				return
			}
			ev = e
		}
		if ev.Request != nil {
			if err := a.socket.Ack(*ev.Request); err != nil {
				a.log.Warn("socket mode ack failed", "error", err)
			}
		}
		ce, ok := translate(ev, a.botID, time.Now())
		if !ok {
			continue
		}
		select {
		case out <- ce:
		case <-ctx.Done():
			return
		}
	}
}

// translate maps a Socket Mode event to a chat event; ok is false for anything ignored.
func translate(ev socketmode.Event, botID string, now time.Time) (integration.ChatEvent, bool) {
	var (
		ce integration.ChatEvent
		ok bool
	)
	switch ev.Type {
	case socketmode.EventTypeEventsAPI:
		ce, ok = fromMessage(ev.Data, botID)
	case socketmode.EventTypeInteractive:
		ce, ok = fromClick(ev.Data)
	case socketmode.EventTypeSlashCommand:
		ce, ok = fromCommand(ev.Data)
	}
	ce.At = now
	return ce, ok
}

func fromMessage(data any, botID string) (integration.ChatEvent, bool) {
	api, ok := data.(slackevents.EventsAPIEvent)
	if !ok {
		return integration.ChatEvent{}, false
	}
	msg, ok := api.InnerEvent.Data.(*slackevents.MessageEvent)
	// Skip our own messages, edits, joins and other subtypes.
	if !ok || msg.User == "" || msg.User == botID || msg.BotID != "" || msg.SubType != "" {
		return integration.ChatEvent{}, false
	}
	thread := msg.ThreadTimeStamp
	if thread == "" {
		thread = msg.TimeStamp
	}
	return integration.ChatEvent{
		Kind: integration.ChatMessage, User: msg.User, Text: msg.Text,
		Thread: integration.MessageRef{Channel: msg.Channel, ID: thread},
	}, true
}

func fromClick(data any) (integration.ChatEvent, bool) {
	cb, ok := data.(slack.InteractionCallback)
	if !ok || cb.Type != slack.InteractionTypeBlockActions || len(cb.ActionCallback.BlockActions) == 0 {
		return integration.ChatEvent{}, false
	}
	act := cb.ActionCallback.BlockActions[0]
	task, version, err := parseButtonValue(act.Value)
	if err != nil {
		return integration.ChatEvent{}, false
	}
	thread := cb.Message.ThreadTimestamp
	if thread == "" {
		thread = cb.Message.Timestamp
	}
	return integration.ChatEvent{
		Kind: integration.ChatButton, User: cb.User.ID, Action: act.ActionID, TaskID: task,
		CardVersion: version, Thread: integration.MessageRef{Channel: cb.Channel.ID, ID: thread},
	}, true
}

func fromCommand(data any) (integration.ChatEvent, bool) {
	cmd, ok := data.(slack.SlashCommand)
	if !ok {
		return integration.ChatEvent{}, false
	}
	return integration.ChatEvent{
		Kind: integration.ChatCommand, User: cmd.UserID, Text: strings.TrimSpace(cmd.Text),
		Thread: integration.MessageRef{Channel: cmd.ChannelID},
	}, true
}
