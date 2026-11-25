package codex

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/ilyachakun/nightshift/schemas"
)

// unsupported are the keywords OpenAI's strict structured output refuses. They stay in the
// original schema, against which the result is validated.
var unsupported = []string{"if", "then", "else", "allOf", "not", "uniqueItems", "$schema", "$id"}

// strictSchema rewrites a result schema into the subset strict structured output accepts:
// self-contained, no conditions, and every property required, with null for an optional one.
func strictSchema(raw []byte) ([]byte, error) {
	flat, err := schemas.Standalone(raw)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(flat, &doc); err != nil {
		return nil, fmt.Errorf("strict schema: %w", err)
	}
	strict(doc)
	return json.Marshal(doc)
}

// strict changes one schema node and the schemas below it in place.
func strict(node any) {
	s, ok := node.(map[string]any)
	if !ok {
		return
	}
	for _, k := range unsupported {
		delete(s, k)
	}
	strict(s["items"])
	if alternatives, ok := s["anyOf"].([]any); ok {
		for _, a := range alternatives {
			strict(a)
		}
	}
	if defs, ok := s["$defs"].(map[string]any); ok {
		for _, d := range defs {
			strict(d)
		}
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		return
	}
	required, _ := s["required"].([]any)
	for name, p := range props {
		strict(p)
		if !slices.Contains(required, any(name)) {
			props[name] = map[string]any{"anyOf": []any{p, map[string]any{"type": "null"}}}
		}
	}
	all := []any{}
	for _, name := range slices.Sorted(maps.Keys(props)) {
		all = append(all, name)
	}
	s["required"], s["additionalProperties"] = all, false
}

// withoutNulls drops the null members strict output writes for the properties it had to
// require, so the result matches the original schema again. A null the original schema
// asks for or allows (a required property, or a type that includes null) stays.
func withoutNulls(raw, original []byte) []byte {
	var doc, schema any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	if flat, err := schemas.Standalone(original); err == nil {
		_ = json.Unmarshal(flat, &schema)
	}
	prune(doc, schema)
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

// prune walks the answer beside its schema. A null is kept only where the schema declares the
// property and requires it or allows null; where the schema says nothing (a union, a reference
// left in place) it is strict mode's filler and goes.
func prune(node, schema any) {
	sc, _ := schema.(map[string]any)
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			prune(item, sc["items"])
		}
	case map[string]any:
		props, _ := sc["properties"].(map[string]any)
		required, _ := sc["required"].([]any)
		for k, item := range v {
			p, declared := props[k]
			if item == nil {
				if !declared || (!slices.Contains(required, any(k)) && !allowsNull(p)) {
					delete(v, k)
				}
				continue
			}
			prune(item, p)
		}
	}
}

// allowsNull reports whether a property schema accepts null itself.
func allowsNull(schema any) bool {
	sc, _ := schema.(map[string]any)
	switch t := sc["type"].(type) {
	case string:
		return t == "null"
	case []any:
		return slices.Contains(t, any("null"))
	}
	if alts, ok := sc["anyOf"].([]any); ok {
		return slices.ContainsFunc(alts, allowsNull)
	}
	return false
}
