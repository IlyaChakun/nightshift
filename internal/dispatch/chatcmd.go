package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/knowledge/memory"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
	"github.com/ilyachakun/nightshift/internal/observability/summary"
)

// statusWindow is how far back `/ns status` looks for finished work.
const statusWindow = 24 * time.Hour

// commandUsage is the answer to an unknown or empty command.
const commandUsage = "Commands: `/ns status`, `/ns memory`, `/ns remember <fact>`, `/ns search <words>`."

// command answers a `/ns` command of the owner in the channel it came from. It
// writes nothing to a task: the answer is one message in the channel, keyed by the command so a
// redelivery does not answer twice.
func (in *Inbox) command(ctx context.Context, t config.Tenant, ev integration.ChatEvent) error {
	verb, arg, _ := strings.Cut(strings.TrimSpace(ev.Text), " ")
	arg = strings.TrimSpace(arg)
	var text string
	var err error
	switch strings.ToLower(verb) {
	case "status":
		text, err = in.statusText(ctx, t)
	case "memory":
		text, err = in.memoryText(ctx, t)
	case "remember":
		text, err = in.remember(ctx, t, ev, arg)
	case "search":
		text, err = in.searchText(ctx, t, arg)
	default:
		text = commandUsage
	}
	if err != nil {
		in.log().WarnContext(ctx, "chat command failed", "verb", verb, "error", err)
		text = "The command failed; see the log."
	}
	card, err := json.Marshal(integration.Card{
		Tenant: t.Name, Kind: integration.CardBlocker, Text: text, Thread: integration.MessageRef{Channel: ev.Thread.Channel},
	})
	if err != nil {
		return fmt.Errorf("inbox: command card: %w", err)
	}
	// Every command is answered: a chat command may carry no id or time, so the key takes the
	// time of arrival unless the adapter set one (a redelivery then repeats it).
	at := ev.At
	if at.IsZero() {
		at = time.Now()
	}
	sum := sha256.Sum256([]byte(ev.User + "\x00" + ev.Text + "\x00" + at.UTC().Format(time.RFC3339Nano)))
	key := "chat-command:" + ev.Thread.Channel + ":" + hex.EncodeToString(sum[:8])
	if err := in.Outbox.Enqueue(ctx, bookdb.New(in.Pool), t.Name, 0, ChatTarget, key, card); err != nil {
		return err
	}
	if _, err := in.Outbox.Flush(ctx); err != nil {
		in.log().WarnContext(ctx, "outbox flush after chat command failed", "error", err)
	}
	return nil
}

func (in *Inbox) statusText(ctx context.Context, t config.Tenant) (string, error) {
	s, err := summary.Build(ctx, in.Pool, t.Name, time.Now().Add(-statusWindow), time.Now())
	if err != nil {
		return "", fmt.Errorf("status: %w", err)
	}
	return slackEscape(s.Format()), nil
}

func (in *Inbox) memoryText(ctx context.Context, t config.Tenant) (string, error) {
	page, err := memory.Store{Pool: in.Pool}.Read(ctx, t.Name)
	if err != nil {
		return "", fmt.Errorf("memory: %w", err)
	}
	now := time.Now()
	var b strings.Builder
	for _, e := range page.Entries {
		if !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(now) {
			continue
		}
		fmt.Fprintf(&b, "- %s (%s)\n", slackEscape(e.Text), slackEscape(e.Source))
	}
	if b.Len() == 0 {
		return "The project memory is empty. Add a fact with `/ns remember <fact>`.", nil
	}
	return "Project memory:\n" + strings.TrimRight(b.String(), "\n"), nil
}

func (in *Inbox) remember(ctx context.Context, t config.Tenant, ev integration.ChatEvent, fact string) (string, error) {
	if fact == "" {
		return "Say what to remember: `/ns remember <fact>`.", nil
	}
	fact = in.Outbox.Mask.String(fact)
	err := memory.Store{Pool: in.Pool}.Remember(ctx, t.Name, fact, "chat:"+ev.User, time.Now())
	switch {
	case errors.Is(err, memory.ErrFull):
		return "The memory page is full: drop or merge an entry first (`nightshift memory show`).", nil
	case errors.Is(err, memory.ErrEntry):
		return "That cannot be remembered: " + err.Error(), nil
	case err != nil:
		return "", fmt.Errorf("remember: %w", err)
	}
	return "Remembered. It lasts 90 days.", nil
}

// slackEscape turns the three characters chat markup reads as control (`<!channel>`, `<url|text>`)
// into entities, so a fact or a repo entry cannot ping a channel or hide a link.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// searchText answers `/ns search <words>` with the best hits of the tenant's book.
func (in *Inbox) searchText(ctx context.Context, t config.Tenant, words string) (string, error) {
	if words == "" {
		return "Say what to look for: `/ns search <words>`.", nil
	}
	hits, err := (&search.Index{Pool: in.Pool}).Search(ctx, search.Query{
		Tenant: t.Name, Text: words, Visible: []string{"all"}, Limit: 5,
	})
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}
	if len(hits) == 0 {
		return "Nothing found for that.", nil
	}
	var b strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&b, "- [%s] %s (%s)\n", slackEscape(h.Kind), slackEscape(h.Title), slackEscape(h.Source))
	}
	return "Book search:\n" + strings.TrimRight(b.String(), "\n"), nil
}
