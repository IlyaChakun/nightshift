// Package config validates YAML configuration files against the JSON Schemas
// in the schemas package.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/schemas"
)

// Kind names a schema, for example "tenant" for schemas/tenant.json.
type Kind string

// Config kinds defined in phase 0.
const (
	KindInstance    Kind = "instance"
	KindProviders   Kind = "providers"
	KindTenant      Kind = "tenant"
	KindProject     Kind = "project"
	KindRole        Kind = "role"
	KindBlastRadius Kind = "blast-radius"
)

const schemaBase = "https://github.com/ilyachakun/nightshift/schemas/"

// Validator compiles schemas on first use and caches them.
type Validator struct {
	once     sync.Once
	compiler *jsonschema.Compiler
	err      error
	mu       sync.Mutex
	compiled map[Kind]*jsonschema.Schema
}

// NewValidator returns a validator backed by the embedded schemas.
func NewValidator() *Validator {
	return &Validator{compiled: map[Kind]*jsonschema.Schema{}}
}

// ValidateYAML parses doc as YAML and validates it against the schema of kind.
func (v *Validator) ValidateYAML(kind Kind, doc []byte) error {
	inst, err := yamlToJSONValue(doc)
	if err != nil {
		return fmt.Errorf("parse %s yaml: %w", kind, err)
	}
	return v.validate(kind, inst)
}

// ValidateJSON validates a JSON document against the schema of kind.
func (v *Validator) ValidateJSON(kind Kind, doc []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return fmt.Errorf("parse %s json: %w", kind, err)
	}
	return v.validate(kind, inst)
}

func (v *Validator) validate(kind Kind, inst any) error {
	sch, err := v.schema(kind)
	if err != nil {
		return err
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	return nil
}

func (v *Validator) schema(kind Kind) (*jsonschema.Schema, error) {
	v.once.Do(func() { v.compiler, v.err = newCompiler() })
	if v.err != nil {
		return nil, v.err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if sch, ok := v.compiled[kind]; ok {
		return sch, nil
	}
	sch, err := v.compiler.Compile(schemaBase + string(kind) + ".json")
	if err != nil {
		return nil, fmt.Errorf("compile schema %q: %w", kind, err)
	}
	v.compiled[kind] = sch
	return sch, nil
}

// newCompiler registers every embedded schema under its $id base URL.
func newCompiler() (*jsonschema.Compiler, error) {
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	entries, err := fs.ReadDir(schemas.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded schemas: %w", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(schemas.FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", e.Name(), err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("parse schema %s: %w", e.Name(), err)
		}
		if err := c.AddResource(schemaBase+e.Name(), doc); err != nil {
			return nil, fmt.Errorf("add schema %s: %w", e.Name(), err)
		}
	}
	return c, nil
}

// yamlToJSONValue turns YAML into the value model jsonschema expects by
// round-tripping through JSON. It rejects multi-document input.
func yamlToJSONValue(doc []byte) (any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(doc))
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("more than one yaml document")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("convert to json: %w", err)
	}
	out, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("reparse json: %w", err)
	}
	return out, nil
}
