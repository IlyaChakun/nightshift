package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/memory"
)

// Event kinds for Lead's memory proposals.
const (
	eventMemoryAccepted = "memory.accepted"
	eventMemoryRefused  = "memory.refused"
)

// minQuoteWords keeps a quote from matching by accident ("ok", "yes").
const minQuoteWords = 4

// memoryProposal is one entry Lead wants on the project memory page.
type memoryProposal struct {
	Text   string `json:"text"`
	Source struct {
		Type string `json:"type"`
		Ref  string `json:"ref"`
	} `json:"source"`
}

// sourceOf checks where a proposal comes from and returns the page source. Only a quote of an
// owner message of this task or a file of the repository qualifies: a ticket, a web page or
// tool output never does, however Lead words it.
func (c *Conductor) sourceOf(ctx context.Context, task bookdb.BookTask, events []bookdb.BookEvent, p memoryProposal) (string, error) {
	switch p.Source.Type {
	case "owner_message":
		quote := strings.ToLower(strings.Join(strings.Fields(p.Source.Ref), " "))
		if len(strings.Fields(quote)) < minQuoteWords {
			return "", fmt.Errorf("the quote has fewer than %d words", minQuoteWords)
		}
		for _, e := range events {
			if e.Kind != "task_created" && e.Kind != "owner_message" {
				continue
			}
			var m struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(e.Payload, &m) == nil && strings.Contains(strings.ToLower(strings.Join(strings.Fields(m.Text), " ")), quote) {
				return "owner:msg-" + strconv.FormatInt(e.ID, 10), nil
			}
		}
		return "", errors.New("the quote is in no owner message of this task")
	case "repo_file":
		mirror, err := c.mirror(ctx, task.Repo)
		if err != nil {
			return "", err
		}
		base, err := c.Git.Rev(ctx, mirror, c.base())
		if err != nil {
			return "", fmt.Errorf("conductor: %w", err)
		}
		if _, found, err := c.Git.Show(ctx, mirror, base, p.Source.Ref); err != nil || !found {
			return "", errors.New("the file is not in the repository")
		}
		return "repo:" + p.Source.Ref, nil
	}
	return "", errors.New("unknown source type")
}

// rememberFromLead writes the proposals that pass the source check to the memory page, one
// event each for accepted and refused ones. A refusal never fails the reply: the owner still
// gets the answer. A lost race on the page is retried once.
func (c *Conductor) rememberFromLead(ctx context.Context, task bookdb.BookTask, raw json.RawMessage) {
	var r struct {
		Memory []memoryProposal `json:"memory"`
	}
	if json.Unmarshal(raw, &r) != nil || len(r.Memory) == 0 {
		return
	}
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		c.log().WarnContext(ctx, "memory proposals skipped", "task", task.ID, "err", err)
		return
	}
	store := memory.Store{Pool: c.Pool}
	for _, p := range r.Memory {
		src, err := c.sourceOf(ctx, task, events, p)
		if err == nil {
			err = c.remember(ctx, store, task.Tenant, p.Text, src)
		}
		if err != nil {
			c.appendEvent(ctx, task, eventMemoryRefused, strings.Join(strings.Fields(p.Text), " ")+": "+err.Error())
			continue
		}
		c.appendEvent(ctx, task, eventMemoryAccepted, strings.Join(strings.Fields(p.Text), " ")+" ("+src+")")
	}
}

func (c *Conductor) remember(ctx context.Context, store memory.Store, tenant, text, source string) error {
	var err error
	for range 2 {
		var page memory.Page
		if page, err = store.Read(ctx, tenant); err != nil {
			return err
		}
		text = strings.Join(strings.Fields(text), " ")
		e := memory.Entry{Key: memory.Key(text), Text: text, Source: source, ExpiresAt: time.Now().Add(memory.DefaultTTL)}
		entries := make([]memory.Entry, 0, len(page.Entries)+1)
		for _, old := range page.Entries {
			if old.Key != e.Key {
				entries = append(entries, old)
			}
		}
		if err = store.Write(ctx, tenant, page.Revision, append(entries, e), "lead", "lead proposal"); !errors.Is(err, memory.ErrStale) {
			return err
		}
	}
	return err
}
