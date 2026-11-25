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
	eventMemoryChecked  = "memory.checked"
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
//
// The stored text must itself be found in the source, so the page holds the owner's own words
// or the file's, never a sentence Lead made up and merely attached a real quote to.
func (c *Conductor) sourceOf(ctx context.Context, task bookdb.BookTask, events []bookdb.BookEvent, p memoryProposal) (string, error) {
	switch p.Source.Type {
	case "owner_message":
		return ownerSource(events, p)
	case "repo_file":
		return c.fileSource(ctx, task, p)
	}
	return "", errors.New("unknown source type")
}

func ownerSource(events []bookdb.BookEvent, p memoryProposal) (string, error) {
	text, quote := normalize(p.Text), normalize(p.Source.Ref)
	if len(strings.Fields(quote)) < minQuoteWords {
		return "", fmt.Errorf("the quote has fewer than %d words", minQuoteWords)
	}
	if !strings.Contains(quote, text) && !strings.Contains(text, quote) {
		return "", errors.New("the text is not the quoted words")
	}
	for _, e := range events {
		if e.Kind != "task_created" && e.Kind != "owner_message" {
			continue
		}
		var m struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &m) == nil && strings.Contains(normalize(m.Text), quote) && strings.Contains(normalize(m.Text), text) {
			return "owner:msg-" + strconv.FormatInt(e.ID, 10), nil
		}
	}
	return "", errors.New("the quote is in no owner message of this task")
}

func (c *Conductor) fileSource(ctx context.Context, task bookdb.BookTask, p memoryProposal) (string, error) {
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return "", err
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return "", fmt.Errorf("conductor: %w", err)
	}
	kind, err := c.Git.ObjectKind(ctx, mirror, base, p.Source.Ref)
	if err != nil || kind != "blob" {
		return "", errors.New("the file is not in the repository")
	}
	body, _, err := c.Git.Show(ctx, mirror, base, p.Source.Ref)
	if err != nil || !strings.Contains(normalize(body), normalize(p.Text)) {
		return "", errors.New("the text is not in the file")
	}
	return "repo:" + p.Source.Ref, nil
}

// normalize lowercases and collapses whitespace, so a quote matches across line breaks.
func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// rememberFromLead writes the proposals that pass the source check to the memory page, one
// event each for accepted and refused ones. A refusal never fails the reply: the owner still
// gets the answer. A lost race on the page is retried once.
func (c *Conductor) rememberFromLead(ctx context.Context, task bookdb.BookTask, jobID int64, raw json.RawMessage) {
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
	// consumeLead repeats after a failure further down: the job's marker keeps the events and
	// the expiry dates from being written twice.
	marker := "job " + strconv.FormatInt(jobID, 10)
	for _, e := range events {
		if e.Kind == eventMemoryChecked && strings.Contains(string(e.Payload), marker) {
			return
		}
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
	c.appendEvent(ctx, task, eventMemoryChecked, marker)
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
