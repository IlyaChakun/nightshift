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
// require, so the result matches the original schema again.
func withoutNulls(raw []byte) []byte {
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return raw
	}
	prune(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	return out
}

func prune(node any) {
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			prune(item)
		}
	case map[string]any:
		for k, item := range v {
			if item == nil {
				delete(v, k)
				continue
			}
			prune(item)
		}
	}
}
