package schemas

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// Standalone rewrites a result schema into one self-contained document for an agent CLI:
// no $schema or $id, and every reference to a sibling file replaced by that schema. A CLI
// resolves neither. Results are still validated against the original.
func Standalone(raw []byte) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("standalone schema: %w", err)
	}
	doc, err := inline(doc, 0)
	if err != nil {
		return nil, fmt.Errorf("standalone schema: %w", err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("standalone schema: %w", err)
	}
	return out, nil
}

// maxDepth stops a cycle of references between files.
const maxDepth = 32

func inline(node any, depth int) (any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("schema nested deeper than %d, a reference cycle?", maxDepth)
	}
	switch v := node.(type) {
	case []any:
		for i, item := range v {
			out, err := inline(item, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = out
		}
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
			return sibling(ref, depth)
		}
		delete(v, "$schema")
		delete(v, "$id")
		for k, item := range v {
			out, err := inline(item, depth+1)
			if err != nil {
				return nil, err
			}
			v[k] = out
		}
	}
	return node, nil
}

// sibling reads the embedded schema a reference names, itself made standalone.
func sibling(ref string, depth int) (any, error) {
	raw, err := fs.ReadFile(FS, strings.TrimPrefix(ref, Base))
	if err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	var target any
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	return inline(target, depth+1)
}
