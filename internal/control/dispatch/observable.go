package dispatch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// notInBrowser matches scenario text that only a check outside the browser can settle: the
// repository against main, file bytes, CI runs, test commands, process output. The Verifier's
// acceptance job has only the browser (ADR 0029), so such a scenario can only escalate the task
// (V4 on the Mac, 2026-11-28: raw bytes, a file against main, CI passing). It matches phrases,
// not single words, so a page named "Pipeline" or a "Commit" button is still a browser scenario.
// The job opens only the preview, so production and the state after a release or a rollback are
// out too (V4 on the Mac, task #1015: "on the production page after the rollback").
var notInBrowser = regexp.MustCompile(`(?i)\b(git (diff|log|show|status)|(against|vs\.?|versus|compared (to|with)) (the )?main\b|main branch|raw bytes|byte order mark|\bbom\b|hexdump|line endings|crlf|ci (passes|is green|run|job|checks?)|github actions|go test|npm test|make (test|verify)|exit code|stdout|stderr|in the shell|server logs?|log files?|` +
	`production (page|site|url|server|environment|instance|revision|deploy(ment)?)|live site|(on|in|to) (the )?production\s*([.,;]|$)|(released|rolled back|deployed) to production|` +
	`post-(release|rollback|deploy(ment)?)|(after|once|when) (the )?(release|rollback|roll-?back|deploy(ment)?) (is |was |has )?(finished|done|completed|over|ended)\b)|` +
	`продакшен|продакшн|(^|\s)(в|на) проде|после (релиза|отката|выкатки|деплоя)|откат (завершён|завершен|закончен)`)

// scenario is one hidden scenario as Lead writes it (schemas/lead-reply.json).
type scenario struct {
	ID    string `json:"id"`
	Given string `json:"given"`
	When  string `json:"when"`
	Then  string `json:"then"`
}

// splitObservable parses the hidden scenarios of a Lead reply and splits them into the ones a
// browser can check and the ids of the others.
func splitObservable(result json.RawMessage) (keep []scenario, out []string) {
	var r struct {
		Scenarios []scenario `json:"hidden_scenarios"`
	}
	if json.Unmarshal(result, &r) != nil {
		return nil, nil
	}
	for _, s := range r.Scenarios {
		if notInBrowser.MatchString(s.Given + "\n" + s.When + "\n" + s.Then) {
			out = append(out, s.ID)
			continue
		}
		keep = append(keep, s)
	}
	return keep, out
}

// eventHiddenSentBack is the one send-back of a spec for scenarios a browser cannot check. It
// is its own kind, so it never takes Lead's one correction for other rules (lead.rejected).
const eventHiddenSentBack = "hidden.sent_back"

// errNotInBrowser is the note Lead gets for scenarios a browser cannot check.
func errNotInBrowser(ids []string) error {
	return fmt.Errorf("hidden scenarios %s cannot be checked in a browser on the preview: the acceptance job has no shell, Git or file access. "+
		"Keep hidden scenarios to what a person sees and does on the preview; move what these check (a diff, file bytes, CI, tests, logs, production or the state after a release or rollback) "+
		"into the acceptance criteria, where review and the checks cover it, and send the spec again", strings.Join(ids, ", "))
}
