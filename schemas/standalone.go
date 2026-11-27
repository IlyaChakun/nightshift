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
	doc, err := inline(doc, nil, 0)
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

// inline rewrites one node. from is the sibling file the node came from, nil for the schema
// itself: a local reference of a sibling would point nowhere once its text is moved, so it is
// replaced too.
func inline(node any, from map[string]any, depth int) (any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("schema nested deeper than %d, a reference cycle?", maxDepth)
	}
	switch v := node.(type) {
	case []any:
		for i, item := range v {
			out, err := inline(item, from, depth+1)
			if err != nil {
				return nil, err
			}
			v[i] = out
		}
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			switch {
			case !strings.HasPrefix(ref, "#"):
				return sibling(ref, depth)
			case from != nil:
				return local(from, ref, depth)
			}
		}
		delete(v, "$schema")
		delete(v, "$id")
		for k, item := range v {
			out, err := inline(item, from, depth+1)
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
	raw, err := fs.ReadFile(files, strings.TrimPrefix(ref, Base))
	if err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	var target map[string]any
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	out, err := inline(target, target, depth+1)
	if m, ok := out.(map[string]any); ok {
		delete(m, "$defs") // every use of them is now in place
	}
	return out, err
}

// local returns a copy of the definition a reference such as #/$defs/list names in doc.
func local(doc map[string]any, ref string, depth int) (any, error) {
	var node any = doc
	for part := range strings.SplitSeq(strings.TrimPrefix(ref, "#/"), "/") {
		m, ok := node.(map[string]any)
		if !ok || m[part] == nil {
			return nil, fmt.Errorf("reference %q points nowhere", ref)
		}
		node = m[part]
	}
	raw, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	var clone any
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil, fmt.Errorf("reference %q: %w", ref, err)
	}
	return inline(clone, doc, depth+1)
}
