package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
)

// maxCoverageMisses keeps the note short; the owner needs the first few, not a list.
const maxCoverageMisses = 6

var (
	numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)
	quotedRe = regexp.MustCompile("[\"«“`]([^\"»”`\\n]{2,40})[\"»”`]")
	// codeLikeRe finds identifiers and codes: a letter-led token with a digit, underscore, dot or
	// slash inside, ALLCAPS words and camelCase words. A plain capitalised word is not one: it is
	// too often just the start of a sentence.
	codeLikeRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[0-9]|[_/][A-Za-z0-9]|\.[A-Za-z0-9])[A-Za-z0-9_./-]*|\b[A-Z]{2,}[0-9]*\b|\b[a-z]+[A-Z][A-Za-z]+\b`)
	// negations a spec must keep: an owner's limit that never reaches the spec is the commonest drop.
	negations = map[string]bool{
		"not": true, "no": true, "never": true, "without": true, "don't": true, "dont": true, "doesn't": true, "cannot": true,
		"не": true, "нет": true, "без": true, "никогда": true, "нельзя": true, "ни": true,
	}
)

// commonAbbreviation are dotted words that are not names.
var commonAbbreviation = map[string]bool{"e.g": true, "i.e": true, "a.m": true, "p.m": true}

// coverageMisses lists what the owner wrote that the spec leaves out: each number, each name or code,
// and a negation when the spec has no out-of-scope entry and no negation of its own. It is a floor,
// not a net: word matching cannot see a changed meaning, and it can raise a false alarm when the
// spec writes a number as a word (docs/research/safety-nets.md, N5).
func coverageMisses(owner []string, spec json.RawMessage) []string {
	var s struct {
		Title    string   `json:"title"`
		Goal     string   `json:"goal"`
		NonGoals []string `json:"non_goals"`
		Criteria []struct {
			Given, When, Then string
		} `json:"acceptance_criteria"`
		Tasks []struct{ Title string } `json:"tasks"`
	}
	if json.Unmarshal(spec, &s) != nil {
		return nil
	}
	parts := []string{s.Title, s.Goal}
	parts = append(parts, s.NonGoals...)
	for _, c := range s.Criteria {
		parts = append(parts, c.Given, c.When, c.Then)
	}
	for _, t := range s.Tasks {
		parts = append(parts, t.Title)
	}
	have := normalize(strings.Join(parts, " "))
	var misses []string
	seen := map[string]bool{}
	miss := func(what string) {
		if k := normalize(what); !seen[k] && len(misses) < maxCoverageMisses {
			seen[k] = true
			misses = append(misses, what)
		}
	}
	text := strings.Join(owner, "\n")
	for _, re := range []*regexp.Regexp{numberRe, codeLikeRe} {
		for _, m := range re.FindAllString(text, -1) {
			m = strings.TrimRight(m, ".-/")
			if m != "" && !commonAbbreviation[strings.ToLower(m)] && !strings.Contains(have, normalize(m)) {
				miss(m)
			}
		}
	}
	for _, m := range quotedRe.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(have, normalize(m[1])) {
			miss(m[1])
		}
	}
	if word := negationIn(text); word != "" && len(s.NonGoals) == 0 && negationIn(have) == "" {
		miss("a limit you stated (\"" + word + "\")")
	}
	return misses
}

// negationIn returns the first negation word of a text, or "".
func negationIn(text string) string {
	for _, f := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		isWord := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'а' && r <= 'я')
		return !isWord && r != '\'' && r != '’'
	}) {
		if negations[strings.ReplaceAll(f, "’", "'")] {
			return f
		}
	}
	return ""
}

// ownerTexts are the owner's messages of a task, oldest first.
func ownerTexts(events []bookdb.BookEvent) []string {
	var out []string
	for _, e := range events {
		if e.Kind != "task_created" && e.Kind != "owner_message" {
			continue
		}
		var m struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(e.Payload, &m) == nil {
			out = append(out, m.Text)
		}
	}
	return out
}

// coverageStep checks a finished spec against the owner's words. The first miss sends the reply back
// to Lead with the list; a second one passes and the owner's card carries the list, so a false alarm
// never blocks a spec twice. It returns the note for the card and whether the reply went back.
func (c *Conductor) coverageStep(ctx context.Context, task bookdb.BookTask, job bookdb.BookJob, spec json.RawMessage) (note string, back bool, err error) {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return "", false, fmt.Errorf("conductor: %w", err)
	}
	misses := coverageMisses(ownerTexts(events), spec)
	if len(misses) == 0 {
		return "", false, nil
	}
	list := strings.Join(misses, "; ")
	if rejectionsAfterOwner(events) < 1 {
		return "", true, c.sendReplyBack(ctx, task, job, fmt.Errorf("the owner's message has words the spec does not carry: %s. Cover each in a criterion or in non_goals, or ask the owner one question about it", list))
	}
	return "\n\nNot found in the spec: " + list + ". Check that nothing you wrote was dropped.", false, nil
}
