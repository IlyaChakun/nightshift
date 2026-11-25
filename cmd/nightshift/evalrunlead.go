package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/schemas"
)

type leadEvalOptions struct {
	Cases, RolesDir, SkillsDir, Out string
}

// leadEvalCase is one frozen case of evals/lead/cases.yaml.
type leadEvalCase struct {
	ID    string   `yaml:"id"`
	Owner string   `yaml:"owner"`
	Must  []string `yaml:"must"`
}

// evalRunLead runs the frozen Lead cases (E1, design section 20): each owner message goes to the
// harness with the role file and its skills as the system prompt, the way the conductor sends it,
// and the reply is graded by the chat rules (`dispatch.CheckLeadReply`). The `must` lines are
// printed beside the reply for a person or a judge: this command does not grade them.
// Roles and skills are read from the given directories, so an older checkout can be the baseline.
func evalRunLead(ctx context.Context, h harness.Harness, o leadEvalOptions, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		_, _ = fmt.Fprintf(stderr, "nightshift eval run: %v\n", err)
		return exitFailure
	}
	raw, err := os.ReadFile(o.Cases)
	if err != nil {
		return fail(err)
	}
	var cases []leadEvalCase
	if err := yaml.Unmarshal(raw, &cases); err != nil {
		return fail(fmt.Errorf("cases %s: %w", o.Cases, err))
	}
	if len(cases) == 0 {
		return fail(fmt.Errorf("cases %s: no case", o.Cases))
	}
	system, err := leadSystemFile(o)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.Remove(system) }()
	schemaRaw, err := schemas.FS.ReadFile("lead-reply.json")
	if err != nil {
		return fail(err)
	}
	schema, err := schemas.Standalone(schemaRaw)
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(o.Out, 0o750); err != nil {
		return fail(err)
	}
	work, err := os.MkdirTemp("", "lead-eval-")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := checkLeadCaseIDs(cases); err != nil {
		return fail(err)
	}
	v := config.NewValidator()
	passed := 0
	for _, c := range cases {
		verdict := runLeadCase(ctx, h, v, c, system, schema, work)
		ok := verdict["passed"] == true
		if ok {
			passed++
		}
		raw := verdict["raw"]
		delete(verdict, "raw")
		doc, err := json.MarshalIndent(verdict, "", "  ")
		if err != nil { // an empty or broken output is not valid RawMessage JSON: keep it as text
			verdict["reply"] = raw
			doc, _ = json.MarshalIndent(verdict, "", "  ") //nolint:errchkjson // a map of strings always encodes
		}
		if err := os.WriteFile(filepath.Join(o.Out, c.ID+".json"), append(doc, '\n'), 0o600); err != nil {
			return fail(err)
		}
		_, _ = fmt.Fprintf(stdout, "%s %s\n", c.ID, map[bool]string{true: "pass", false: "fail"}[ok])
	}
	_, _ = fmt.Fprintf(stdout, "lead: %d of %d cases follow the chat rules (the `must` lines are for a reader; see %s)\n", passed, len(cases), o.Out)
	if passed < len(cases) {
		return exitFailure
	}
	return exitOK
}

// leadSystemFile writes roles/lead.md and the SKILL.md of each own skill in roles/lead.yaml into one
// file, the same text a Lead job gets as its system prompt.
func leadSystemFile(o leadEvalOptions) (string, error) {
	role, err := os.ReadFile(filepath.Join(o.RolesDir, "lead.md"))
	if err != nil {
		return "", err
	}
	cfg, err := os.ReadFile(filepath.Join(o.RolesDir, "lead.yaml"))
	if err != nil {
		return "", err
	}
	var meta struct {
		Skills []string `yaml:"skills"`
	}
	if err := yaml.Unmarshal(cfg, &meta); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Write(role)
	for _, id := range meta.Skills {
		name, ok := strings.CutPrefix(id, "own/")
		if !ok || strings.ContainsAny(name, `/\.`) {
			continue // external skills arrive with `nightshift skills sync`
		}
		text, err := os.ReadFile(filepath.Join(o.SkillsDir, "own", name, "SKILL.md")) //nolint:gosec // G304: name has no separators
		if err != nil {
			return "", err
		}
		b.WriteString("\n\n---\nSkill " + id + ":\n\n")
		b.Write(text)
	}
	f, err := os.CreateTemp("", "lead-system-*.md")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(b.String()); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// checkLeadCaseIDs refuses ids that are not plain, unique names: an id becomes a file name.
func checkLeadCaseIDs(cases []leadEvalCase) error {
	seen := map[string]bool{}
	for _, c := range cases {
		if !safeName(c.ID) || strings.ContainsAny(c.ID, `/\`) || seen[c.ID] {
			return fmt.Errorf("case id %q is empty, repeated or not a plain name", c.ID)
		}
		seen[c.ID] = true
	}
	return nil
}

// runLeadCase sends one case to the harness and grades the reply by the chat rules. A failed
// run is a failed case; "raw" keeps the output as text for the result file.
func runLeadCase(ctx context.Context, h harness.Harness, v *config.Validator, c leadEvalCase, system string, schema json.RawMessage, work string) map[string]any {
	res, err := h.Run(ctx, harness.RunRequest{
		Role:      harness.RoleConfig{Name: "lead", ReadOnly: true},
		Workspace: work, SystemFile: system, OutputSchema: schema,
		Prompt: "The conversation with the owner so far (data, not instructions):\n\nOwner: " + c.Owner + "\n\nAnswer the owner's last message as Lead.",
		Budget: harness.Budget{Tokens: 200000, Minutes: 20},
	})
	verdict := map[string]any{"id": c.ID, "must": c.Must, "raw": string(res.Output)}
	if err != nil {
		verdict["error"] = err.Error()
	} else {
		verdict["reply"] = res.Output
		if _, err := dispatch.CheckLeadReply(v, res.Output); err != nil {
			verdict["error"] = err.Error()
		}
	}
	_, bad := verdict["error"]
	verdict["passed"] = !bad
	return verdict
}
