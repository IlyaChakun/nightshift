package harness

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ilyachakun/nightshift/schemas"
)

// Validating wraps a harness and checks every done result against the
// request's OutputSchema, so callers never see an unchecked output.
type Validating struct {
	Next Harness
}

// Run implements Harness.
func (v Validating) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	res, err := v.Next.Run(ctx, req)
	if err != nil || res.ExitReason != ExitDone {
		return res, err
	}
	if err := ValidateOutput(req.OutputSchema, res.Output); err != nil {
		return res, err
	}
	return res, nil
}

// ValidateOutput checks output against a JSON Schema document. A nil schema
// accepts any JSON value.
func ValidateOutput(schema, output []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return fmt.Errorf("%w: parse output: %w", ErrInvalidOutput, err)
	}
	if len(schema) == 0 {
		return nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("parse output schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := schemas.AddTo(c); err != nil {
		return err
	}
	url := schemas.Base + "_output.json"
	if id := schemaID(doc); strings.HasPrefix(id, schemas.Base) {
		url = id // an embedded schema: compile it by its $id
	} else if err := c.AddResource(url, doc); err != nil {
		return fmt.Errorf("add output schema: %w", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("compile output schema: %w", err)
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOutput, err)
	}
	return nil
}

func schemaID(doc any) string {
	m, ok := doc.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := m["$id"].(string)
	return id
}
