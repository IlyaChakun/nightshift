package dispatch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// notInBrowser matches scenario text that a person in a browser on the preview cannot check:
// the repository and its diff, file bytes, CI and tests, logs and the shell. The Verifier's
// acceptance job has only the browser (ADR 0029), so such a scenario can only escalate the task
// (V4 on the Mac, 2026-11-28: raw bytes, a file against main, CI passing). The list is narrow on
// purpose: a false match costs Lead one more turn, a missed one an escalation.
var notInBrowser = regexp.MustCompile(`(?i)\b(git|diff|commits?|main branch|against main|vs\.? main|bom|byte order|raw bytes|hexdump|line endings?|crlf|ci|pipeline|github actions|unit tests?|test suite|go test|npm test|stdout|stderr|exit code|shell|log files?|pull request|repository)\b`)

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

// errNotInBrowser is the note Lead gets for scenarios a browser cannot check.
func errNotInBrowser(ids []string) error {
	return fmt.Errorf("hidden scenarios %s cannot be checked in a browser on the preview: the acceptance job has no shell, Git or file access. "+
		"Keep hidden scenarios to what a person sees and does on the preview; move what these check (a diff, file bytes, CI, tests, logs) "+
		"into the acceptance criteria, where review and the checks cover it, and send the spec again", strings.Join(ids, ", "))
}
