package summary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// ErrNoTask means the tenant has no task with that id.
var ErrNoTask = errors.New("no such task")

// Entry is one line of a task's timeline: when, who, what.
type Entry struct {
	At   time.Time
	Who  string
	What string
}

// Timeline reads a task's events and jobs from the book and returns them in time order.
func Timeline(ctx context.Context, pool *pgxpool.Pool, tenant string, taskID int64) ([]Entry, error) {
	q := bookdb.New(pool)
	task, err := q.GetTask(ctx, taskID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && task.Tenant != tenant) {
		return nil, fmt.Errorf("summary: task #%d: %w", taskID, ErrNoTask)
	}
	if err != nil {
		return nil, fmt.Errorf("summary: task #%d: %w", taskID, err)
	}
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: taskID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("summary: events: %w", err)
	}
	jobs, err := q.AllTaskJobs(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("summary: jobs: %w", err)
	}
	return BuildTimeline(events, jobs), nil
}

// maxDetail bounds the text of one line, so a long error does not flood the terminal.
const maxDetail = 160

// BuildTimeline merges events and jobs into one list in time order. Each job gives a line when
// it was queued and one when it ended, with its exit, time, tokens, cost and, unless it was
// done, the reason from its result.
func BuildTimeline(events []bookdb.BookEvent, jobs []bookdb.BookJob) []Entry {
	var out []Entry
	for _, e := range events {
		out = append(out, Entry{At: e.CreatedAt.Time, Who: e.Actor, What: eventText(e)})
	}
	for _, j := range jobs {
		name := fmt.Sprintf("job %d", j.ID)
		out = append(out, Entry{At: j.CreatedAt.Time, Who: j.Role, What: fmt.Sprintf("%s queued on %s", name, j.Provider)})
		if j.FinishedAt.Valid {
			out = append(out, Entry{At: j.FinishedAt.Time, Who: j.Role, What: name + " " + jobEnd(j)})
		} else if j.State == "running" && j.StartedAt.Valid {
			out = append(out, Entry{At: j.StartedAt.Time, Who: j.Role, What: fmt.Sprintf("%s running, attempt %d", name, j.Attempt)})
		} else if j.State != "queued" && j.State != "running" {
			// A canceled job keeps no end time: say so at the last time it has.
			when := j.CreatedAt.Time
			if j.StartedAt.Valid {
				when = j.StartedAt.Time
			}
			out = append(out, Entry{At: when, Who: j.Role, What: name + " " + j.State + " (no end time)"})
		}
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return a.At.Compare(b.At) })
	return out
}

func eventText(e bookdb.BookEvent) string {
	var p map[string]any
	_ = json.Unmarshal(e.Payload, &p)
	if e.Kind == "task.moved" {
		return fmt.Sprintf("%v -> %v (%v)", p["from"], p["to"], p["event"])
	}
	if len(p) == 0 {
		return e.Kind
	}
	return clip(e.Kind + " " + string(e.Payload))
}

func jobEnd(j bookdb.BookJob) string {
	exit := j.State
	if j.ExitReason.Valid {
		exit = j.ExitReason.String
	}
	parts := []string{exit}
	if j.StartedAt.Valid {
		took := "after " + j.FinishedAt.Time.Sub(j.StartedAt.Time).Round(time.Second).String()
		if j.Attempt > 1 {
			// A requeued job keeps its first start, so the time covers every attempt.
			took += fmt.Sprintf(" over %d attempts", j.Attempt)
		}
		parts = append(parts, took)
	}
	var u struct {
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
	}
	if json.Unmarshal(j.Usage, &u) == nil && u.InputTokens+u.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("%d tokens", u.InputTokens+u.OutputTokens))
	}
	if u.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", u.CostUSD))
	}
	text := strings.Join(parts, ", ")
	if exit != "done" {
		if why := reason(j.Result); why != "" {
			text += ": " + why
		}
	}
	return clip(text)
}

// reason picks the why of a job that did not finish well: the supervisor's stop or the
// harness error, else nothing.
func reason(result []byte) string {
	var r map[string]any
	if json.Unmarshal(result, &r) != nil {
		return ""
	}
	for _, k := range []string{"stopped", "error"} {
		if s, ok := r[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxDetail {
		return s
	}
	return strings.ToValidUTF8(s[:maxDetail], "") + "..."
}

// FormatTimeline renders a timeline, one line per entry, in the given time zone.
func FormatTimeline(taskID int64, entries []Entry, loc *time.Location) string {
	if len(entries) == 0 {
		return fmt.Sprintf("task #%d has no events yet\n", taskID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "task #%d\n", taskID)
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  %-10s %s\n", e.At.In(loc).Format("2006-01-02 15:04:05"), e.Who, e.What)
	}
	return b.String()
}
