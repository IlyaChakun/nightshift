package composelocal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// LintSource reads the recipe file as it is written, before the engine's CLI renders it. The
// render follows every reference to another file, so by the time Lint sees the result the other
// file has already been read and merged, and Lint cannot tell where a setting came from. The
// recipe is therefore one file: `include`, `extends` with anything but a service of the same
// file, and `label_file` are refused. So is a key that hides its name behind a tag, and a recipe
// that is not YAML this code can walk.
func LintSource(src []byte) []Violation {
	var out []Violation
	top := func(rule, detail string) {
		out = append(out, Violation{Service: "(recipe file)", Rule: rule, Detail: detail})
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	docs := 0
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			top("recipe is not readable YAML", err.Error())
			return out
		}
		docs++
		root := deref(&doc)
		if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
			root = deref(root.Content[0])
		}
		if root.Kind != yaml.MappingNode {
			top("recipe is not readable YAML", "the document is not a mapping")
			continue
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			switch key := deref(root.Content[i]); {
			case key.Kind != yaml.ScalarNode:
				top("recipe is not readable YAML", "a top-level key is not a plain name")
			case key.Value == "include":
				top("another compose file", "include")
			case key.Value == "<<":
				// A merge could bring `include` in under a name this walk does not see.
				top("another compose file", "a merge key at the top level")
			}
		}
		walkKeys(root, 0, map[*yaml.Node]bool{}, top)
	}
	if docs == 0 {
		top("recipe is not readable YAML", "the file is empty")
	}
	return out
}

// deref follows aliases to the node they name; a chain longer than any honest file has ends at
// the alias itself, which no rule accepts.
func deref(n *yaml.Node) *yaml.Node {
	for i := 0; i < 16 && n.Kind == yaml.AliasNode && n.Alias != nil; i++ {
		n = n.Alias
	}
	return n
}

// plainKeyTags are the tags under which a key means the text that is written.
var plainKeyTags = map[string]bool{"!!str": true, "!!int": true, "!!float": true, "!!bool": true, "!!null": true}

// maxDepth bounds the recursion of the walk.
const maxDepth = 64

// walkKeys refuses every key that could name another file, and every key whose tag makes the
// engine read a name other than the one written (a binary key decodes to any name, a key tagged
// as a merge merges under any name); only the tags of plain text and numbers are let through. The keys are looked for at any depth, because anchors and
// merge keys can place a service's settings anywhere in the file; the price is that a service or
// a variable cannot be called `extends` or `label_file`.
// Each node is visited once, so a file of aliases that expands to a huge tree stays cheap.
func walkKeys(n *yaml.Node, depth int, seen map[*yaml.Node]bool, report func(rule, detail string)) {
	n = deref(n)
	if seen[n] {
		return
	}
	seen[n] = true
	if depth > maxDepth {
		report("recipe is not readable YAML", "nested too deep")
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := deref(n.Content[i])
			if key.Kind != yaml.ScalarNode {
				continue
			}
			switch tag := key.ShortTag(); {
			case !plainKeyTags[tag] && (tag != "!!merge" || key.Value != "<<"):
				report("recipe key hides its name", tag)
			case key.Value == "extends" && !sameFileExtends(n.Content[i+1]):
				report("another compose file", "extends")
			case key.Value == "label_file":
				report("another file", "label_file")
			}
		}
	}
	for _, c := range n.Content {
		walkKeys(c, depth+1, seen, report)
	}
}

// sameFileExtends accepts the two forms that name a service of this file and nothing else: a
// plain name, or a mapping whose only key is `service` with a plain name.
func sameFileExtends(v *yaml.Node) bool {
	v = deref(v)
	if v.Kind == yaml.ScalarNode {
		return true
	}
	if v.Kind != yaml.MappingNode || len(v.Content) != 2 {
		return false
	}
	key, val := deref(v.Content[0]), deref(v.Content[1])
	return key.Kind == yaml.ScalarNode && key.Value == "service" && val.Kind == yaml.ScalarNode
}

// lintFiles checks the files the engine's CLI reads by itself while it renders: the recipe and
// the project's `.env`. The checkout is the change's, so either could be a link out of it.
func lintFiles(tree Tree) ([]Violation, error) {
	root := resolve(filepath.Clean(tree.Dir))
	var out []Violation
	for _, p := range []string{tree.Recipe, filepath.Join(tree.Dir, ".env")} {
		if !inside(root, p) {
			out = append(out, Violation{Service: "(recipe file)", Rule: "file outside the checkout", Detail: p})
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	src, err := os.ReadFile(tree.Recipe)
	if err != nil {
		return nil, fmt.Errorf("composelocal: read the recipe: %w", err)
	}
	return LintSource(src), nil
}
