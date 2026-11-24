// Package check turns tool reports into findings and compares them with a baseline
// (design section 8). It only parses and compares; running tools is the job of the command.
package check

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Severity of a finding, as in schemas/finding.json.
type Severity string

// Severities: only block stops a task.
const (
	Block Severity = "block"
	Warn  Severity = "warn"
)

// Location points at a file and, when the tool knows it, lines.
type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// Finding is one problem from a tool; it serializes to schemas/finding.json.
type Finding struct {
	RuleID      string    `json:"rule_id"`
	Category    string    `json:"category"`
	Severity    Severity  `json:"severity"`
	LayerCaught string    `json:"layer_caught"`
	Message     string    `json:"message"`
	Location    *Location `json:"location,omitempty"`
	Fingerprint string    `json:"fingerprint"`
}

type sarifLog struct {
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name string `json:"name"`
			} `json:"driver"`
		} `json:"tool"`
		Results []sarifResult `json:"results"`
	} `json:"runs"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		Physical struct {
			Artifact struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
				EndLine   int `json:"endLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

// ErrSARIF means the input is not a SARIF 2.1.0 log.
var ErrSARIF = errors.New("check: invalid SARIF")

// maxMessage keeps a finding within the schema's limit.
const maxMessage = 2000

var unsafeRule = regexp.MustCompile(`[^a-z0-9._-]+`)

// ParseSARIF reads a SARIF 2.1.0 log and returns its results as findings of the given
// category and layer. A result without a rule id gets the rule id "new".
func ParseSARIF(r io.Reader, category, layer string) ([]Finding, error) {
	var log sarifLog
	if err := json.NewDecoder(r).Decode(&log); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSARIF, err)
	}
	if log.Version != "2.1.0" {
		return nil, fmt.Errorf("%w: version %q, want 2.1.0", ErrSARIF, log.Version)
	}
	var out []Finding
	for _, run := range log.Runs {
		for _, res := range run.Results {
			out = append(out, toFinding(res, run.Tool.Driver.Name, category, layer))
		}
	}
	return out, nil
}

func toFinding(res sarifResult, tool, category, layer string) Finding {
	f := Finding{
		RuleID:      ruleID(res.RuleID, tool),
		Category:    category,
		Severity:    Warn,
		LayerCaught: layer,
		Message:     truncate(res.Message.Text, maxMessage),
	}
	if f.Message == "" {
		f.Message = "no message from " + tool
	}
	if res.Level == "error" || res.Level == "" { // SARIF's default level is warning, but tools that omit it mean a failure
		f.Severity = Block
	}
	if len(res.Locations) > 0 {
		p := res.Locations[0].Physical
		if p.Artifact.URI != "" {
			f.Location = &Location{Path: strings.TrimPrefix(p.Artifact.URI, "file://"), StartLine: p.Region.StartLine, EndLine: p.Region.EndLine}
		}
	}
	f.Fingerprint = fingerprint(f, res.PartialFingerprints)
	return f
}

func ruleID(id, tool string) string {
	id = strings.Trim(unsafeRule.ReplaceAllString(strings.ToLower(id), "-"), "-._")
	if id == "" {
		return "new"
	}
	if tool = strings.ToLower(tool); tool != "" && !strings.HasPrefix(id, tool) {
		if t := strings.Trim(unsafeRule.ReplaceAllString(tool, "-"), "-"); t != "" {
			return t + "." + id
		}
	}
	return id
}

// fingerprint is stable when lines move: it uses the tool's own fingerprint when there is
// one, otherwise the rule, the file and the message, never the line numbers.
func fingerprint(f Finding, partial map[string]string) string {
	h := sha256.New()
	if len(partial) > 0 {
		for _, k := range slices.Sorted(maps.Keys(partial)) {
			fmt.Fprintf(h, "%s=%s\n", k, partial[k])
		}
	} else {
		path := ""
		if f.Location != nil {
			path = f.Location.Path
		}
		fmt.Fprintf(h, "%s\n%s\n%s", f.RuleID, path, f.Message)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// NewViolations returns the findings of current that the baseline does not have, matched
// by fingerprint. Each baseline entry cancels one current finding, so a second copy of an
// old violation still counts as new.
func NewViolations(baseline, current []Finding) []Finding {
	seen := map[string]int{}
	for _, f := range baseline {
		seen[f.Fingerprint]++
	}
	var out []Finding
	for _, f := range current {
		if seen[f.Fingerprint] > 0 {
			seen[f.Fingerprint]--
			continue
		}
		out = append(out, f)
	}
	return out
}

// Blocking reports whether any finding stops the task.
func Blocking(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Block {
			return true
		}
	}
	return false
}
