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

// files holds every *.json schema in this directory.
//
//go:embed *.json
var files embed.FS

// FS returns every *.json schema in this directory. It is a function, so no importer can
// swap the schemas that results are validated against.
func FS() embed.FS { return files }

// Base is the $id prefix of every embedded schema, so one schema can refer to a sibling
// such as finding.json.
const Base = "https://github.com/ilyachakun/nightshift/schemas/"

// AddTo registers every embedded schema with c under Base plus its file name.
func AddTo(c *jsonschema.Compiler) error {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read embedded schemas: %w", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(files, e.Name())
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
