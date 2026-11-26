package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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
		"never": true, "without": true, "don't": true, "dont": true, "без": true, "нельзя": true, "никогда": true,
	}
	urlRe = regexp.MustCompile(`https?://\S+`)
	// bareAnswerRe is an owner message that only picks an option ("2", "option 2").
	bareAnswerRe = regexp.MustCompile(`^[^\pL\pN]*(?:option\s*|вариант\s*)?\d+[^\pL\pN]*$`)
)

// normNumber drops a thousands separator so "1,000" and "1000" match.
func normNumber(n string) string {
	return thousandsRe.ReplaceAllString(n, "$1$2")
}

var thousandsRe = regexp.MustCompile(`(\d),(\d{3})\b`)

// commonAbbreviation are dotted words that are not names.
var commonAbbreviation = map[string]bool{"e.g": true, "i.e": true, "a.m": true, "p.m": true}

// coverageMisses lists what the owner wrote that the spec leaves out: each number, each name or code,
// and a negation when the spec has no out-of-scope entry and no negation of its own. It is a floor,
// not a net: word matching cannot see a changed meaning, and it can raise a false alarm when the
// spec writes a number as a word (docs/research/safety-nets.md, N5).
func coverageMisses(owner []string, spec json.RawMessage) []string {
	have, nonGoals, ok := specHaystack(spec)
	if !ok {
		return nil
	}
	haveNumbers := map[string]bool{}
	for _, n := range numberRe.FindAllString(have, -1) {
		haveNumbers[n] = true
	}
	var misses []string
	seen := map[string]bool{}
	miss := func(what string) {
		if k := normalize(what); !seen[k] && len(misses) < maxCoverageMisses {
			seen[k] = true
			misses = append(misses, what)
		}
	}
	var kept []string
	for _, m := range owner {
		if !bareAnswerRe.MatchString(m) {
			kept = append(kept, urlRe.ReplaceAllString(m, " "))
		}
	}
	text := strings.Join(kept, "\n")
	// A number inside an identifier ("report_v2", "go1.25") belongs to the name, not to the numbers.
	for _, n := range numberRe.FindAllString(normNumber(codeLikeRe.ReplaceAllString(text, " ")), -1) {
		if !haveNumbers[n] {
			miss(n)
		}
	}
	for _, m := range codeLikeRe.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".-/")
		if m != "" && !commonAbbreviation[strings.ToLower(m)] && !strings.Contains(have, normalize(m)) {
			miss(m)
		}
	}
	for _, m := range quotedRe.FindAllStringSubmatch(text, -1) {
		if !strings.Contains(have, normalize(m[1])) {
			miss(m[1])
		}
	}
	if word := negationIn(text); word != "" && nonGoals == 0 && negationIn(have) == "" {
		miss("a limit you stated (\"" + word + "\")")
	}
	return misses
}

// specHaystack joins the parts of a spec that carry the owner's words, normalised, and counts its
// non-goals. ok is false when the spec does not parse.
func specHaystack(spec json.RawMessage) (have string, nonGoals int, ok bool) {
	var s struct {
		Title    string   `json:"title"`
		Goal     string   `json:"goal"`
		NonGoals []string `json:"non_goals"`
		PRD      string   `json:"prd_path"`
		ADRs     []string `json:"adr_paths"`
		Criteria []struct {
			Given, When, Then string
			Quote             string `json:"source_quote"`
		} `json:"acceptance_criteria"`
		Tasks []struct{ Title, Repo string } `json:"tasks"`
	}
	if json.Unmarshal(spec, &s) != nil {
		return "", 0, false
	}
	parts := append([]string{s.Title, s.Goal, s.PRD}, s.NonGoals...)
	parts = append(parts, s.ADRs...)
	for _, c := range s.Criteria {
		parts = append(parts, c.Given, c.When, c.Then, c.Quote)
	}
	for _, t := range s.Tasks {
		parts = append(parts, t.Title, t.Repo)
	}
	have = normNumber(normalize(strings.Join(parts, " ")))
	return have, len(s.NonGoals), true
}

// negationIn returns the first negation word of a text, or "".
func negationIn(text string) string {
	if strings.Contains(strings.ToLower(text), "do not") {
		return "do not"
	}
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
	if softRejectionsAfterOwner(events) < 1 {
		return "", true, c.sendReplyBackSoft(ctx, task, job, fmt.Errorf("the owner's message has words the spec does not carry: %s. Cover each in a criterion or in non_goals, or ask the owner one question about it; if the owner changed their mind later, say so in the goal", list))
	}
	return "\n\nNot found in the spec: " + list + ". Check that nothing you wrote was dropped.", false, nil
}

// sendReplyBackSoft sends a reply back for the coverage check. It is recorded as a soft rejection:
// it never uses up the one correction that the other checks get, and it never ends the job.
func (c *Conductor) sendReplyBackSoft(ctx context.Context, task bookdb.BookTask, job bookdb.BookJob, why error) error {
	q := bookdb.New(c.Pool)
	payload, _ := json.Marshal(map[string]any{"text": why.Error(), "soft": true}) //nolint:errchkjson // a string map always encodes
	if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
		Tenant: task.Tenant, TaskID: pgtype.Int8{Int64: task.ID, Valid: true}, Kind: eventLeadRejected, Actor: "system", Payload: payload,
	}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	_, err := q.ConsumeJob(ctx, job.ID)
	return ignoreNoRows(err)
}

// isSoft reports whether a rejection event came from the coverage check.
func isSoft(e bookdb.BookEvent) bool {
	var p struct {
		Soft bool `json:"soft"`
	}
	return json.Unmarshal(e.Payload, &p) == nil && p.Soft
}

// softRejectionsAfterOwner counts the coverage send-backs since the owner's last message.
func softRejectionsAfterOwner(events []bookdb.BookEvent) int {
	n := 0
	for _, e := range slices.Backward(events) {
		switch {
		case e.Kind == "task_created" || e.Kind == "owner_message" || e.Kind == eventResearchRequested:
			return n
		case e.Kind == eventLeadRejected && isSoft(e):
			n++
		}
	}
	return n
}
