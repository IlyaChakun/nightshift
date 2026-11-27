package rubric

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
)

// answerSchema is the reply format of a judge turn. It lives here, not in schemas/, because it
// is an internal contract between this package and the harness, not a stored record.
const answerSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["answer", "rationale", "evidence"],
  "properties": {
    "answer": {"enum": ["pass", "fail", "unknown"]},
    "rationale": {"type": "string", "maxLength": 1500},
    "evidence": {"type": "array", "items": {"type": "string"}, "maxItems": 5}
  }
}`

// HarnessJudge asks a model, through the Harness interface, one rubric question per turn. The
// role is read-only and gets no skills; the document is data in the prompt.
type HarnessJudge struct {
	Harness harness.Harness
	// Role is the harness role name; empty means "rubric-judge".
	Role string
	// Workspace is an empty directory the turn runs in, so the judge sees no project files.
	// Empty means a fresh temporary directory per call.
	Workspace string
	Budget    harness.Budget
	Env       map[string]string
}

// Judge implements Judge.
func (j HarnessJudge) Judge(ctx context.Context, r Request) (Answer, error) {
	if j.Harness == nil {
		return Answer{}, errors.New("rubric: no harness")
	}
	ws := j.Workspace
	if ws == "" {
		dir, err := os.MkdirTemp("", "rubric-*")
		if err != nil {
			return Answer{}, fmt.Errorf("rubric: %w", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		ws = dir
	}
	role := j.Role
	if role == "" {
		role = "rubric-judge"
	}
	res, err := j.Harness.Run(ctx, harness.RunRequest{
		Role:         harness.RoleConfig{Name: role, ReadOnly: true},
		Workspace:    ws,
		Prompt:       r.Prompt(),
		OutputSchema: json.RawMessage(answerSchema),
		Budget:       j.Budget,
		Env:          j.Env,
	})
	if err != nil {
		return Answer{}, fmt.Errorf("rubric: %s: %w", r.Card.ID, err)
	}
	if res.ExitReason != "" && res.ExitReason != harness.ExitDone {
		return Answer{}, fmt.Errorf("rubric: %s: the judge turn ended with %q", r.Card.ID, res.ExitReason)
	}
	var a struct {
		Answer    string   `json:"answer"`
		Rationale string   `json:"rationale"`
		Evidence  []string `json:"evidence"`
	}
	dec := json.NewDecoder(bytes.NewReader(res.Output))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return Answer{}, fmt.Errorf("rubric: %s: %w", r.Card.ID, err)
	}
	return Answer{Answer: a.Answer, Rationale: a.Rationale, Evidence: a.Evidence}, nil
}
