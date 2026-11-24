// Package schemas embeds the JSON Schemas for configs and role results so the
// binary validates them without reading files at runtime.
package schemas

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// FS holds every *.json schema in this directory.
//
//go:embed *.json
var FS embed.FS

// Base is the $id prefix of every embedded schema, so one schema can refer to a sibling
// such as finding.json.
const Base = "https://github.com/ilyachakun/nightshift/schemas/"

// AddTo registers every embedded schema with c under Base plus its file name.
func AddTo(c *jsonschema.Compiler) error {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return fmt.Errorf("read embedded schemas: %w", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(FS, e.Name())
		if err != nil {
			return fmt.Errorf("read schema %s: %w", e.Name(), err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("parse schema %s: %w", e.Name(), err)
		}
		if err := c.AddResource(Base+e.Name(), doc); err != nil {
			return fmt.Errorf("add schema %s: %w", e.Name(), err)
		}
	}
	return nil
}
