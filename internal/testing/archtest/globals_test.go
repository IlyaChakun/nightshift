package archtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mutableVars returns "file:line Name" for each exported package-level variable in src that is
// not a sentinel error. Any importer could change such a variable for the whole process; a
// package exports a function that returns a fresh value instead, as flow.All does
// (docs/research/go-quality-audit.md, F7).
func mutableVars(fset *token.FileSet, name, src string) ([]string, error) {
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	std := importNames(f)
	var out []string
	for _, decl := range f.Decls {
		g, ok := decl.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, spec := range g.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if !n.IsExported() || i < len(vs.Values) && sentinelError(vs.Values[i], std) {
					continue
				}
				out = append(out, fset.Position(n.Pos()).String()+" "+n.Name)
			}
		}
	}
	return out, nil
}

// importNames maps the local name of each import of f to its path.
func importNames(f *ast.File) map[string]string {
	names := map[string]string{}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := path
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = path
	}
	return names
}

// sentinelError reports whether e is errors.New(...) or fmt.Errorf(...) of the standard library:
// an error value callers compare with errors.Is, which the Go style allows. imports maps each
// local import name of the file to its path.
func sentinelError(e ast.Expr, imports map[string]string) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch imports[pkg.Name] + "." + sel.Sel.Name {
	case "errors.New", "fmt.Errorf":
		return true
	}
	return false
}

// TestNoExportedMutableVars runs the check on every production Go file of the module.
func TestNoExportedMutableVars(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var found []string
	files := 0
	for _, root := range []string{"../../.."} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && slices.Contains([]string{"testdata", ".git", "node_modules"}, d.Name()) {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path) //nolint:gosec // files of this repository
			if err != nil {
				return err
			}
			files++
			got, err := mutableVars(fset, path, string(src))
			found = append(found, got...)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 100 {
		t.Fatalf("read %d Go files; the walk is incomplete", files)
	}
	for _, v := range found {
		t.Errorf("%s: an exported package variable is mutable policy for every importer; unexport it or export a function that returns a fresh value", v)
	}
}

// TestMutableVarsCatches runs the check on small sources, so a broken check cannot pass the
// real tree silently.
func TestMutableVarsCatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{"exported slice fails", "package p\n\nvar Kinds = []string{\"a\"}\n", []string{"p.go:3:5 Kinds"}},
		{"exported var in a group fails", "package p\n\nvar (\n\tA = 1\n\tb = 2\n)\n", []string{"p.go:4:2 A"}},
		{"declared without a value fails", "package p\n\nvar Hook func()\n", []string{"p.go:3:5 Hook"}},
		{"sentinel errors pass", "package p\n\nimport (\n\t\"errors\"\n\t\"fmt\"\n)\n\nvar ErrA = errors.New(\"a\")\nvar ErrB = fmt.Errorf(\"b: %w\", ErrA)\n", nil},
		{"several names in one spec", "package p\n\nimport \"errors\"\n\nvar ErrA, B = errors.New(\"a\"), []int{}\n", []string{"p.go:5:11 B"}},
		{"errors.New of another package fails", "package p\n\nimport errors \"example.com/pkgerrors\"\n\nvar ErrA = errors.New(\"a\")\n", []string{"p.go:5:5 ErrA"}},
		{"an error from a call of the package fails", "package p\n\nvar ErrA error = newErr()\n\nfunc newErr() error { return nil }\n", []string{"p.go:3:5 ErrA"}},
		{"unexported var passes", "package p\n\nvar kinds = []string{\"a\"}\n", nil},
		{"constants and functions pass", "package p\n\nconst Max = 3\n\nfunc Kinds() []string { return []string{\"a\"} }\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mutableVars(token.NewFileSet(), "p.go", tt.src)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
