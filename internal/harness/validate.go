package harness

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ilyachakun/nightshift/schemas"
)

// schemaBase matches the $id prefix of the embedded schemas, so a role schema
// passed as OutputSchema can refer to siblings such as finding.json.
const schemaBase = "https://github.com/ilyachakun/nightshift/schemas/"

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
	if err := addEmbedded(c); err != nil {
		return err
	}
	url := schemaBase + "_output.json"
	if id := schemaID(doc); strings.HasPrefix(id, schemaBase) {
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

func addEmbedded(c *jsonschema.Compiler) error {
	entries, err := fs.ReadDir(schemas.FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded schemas: %w", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(schemas.FS, e.Name())
		if err != nil {
			return fmt.Errorf("read schema %s: %w", e.Name(), err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("parse schema %s: %w", e.Name(), err)
		}
		if err := c.AddResource(schemaBase+e.Name(), doc); err != nil {
			return fmt.Errorf("add schema %s: %w", e.Name(), err)
		}
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
