package archtest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestWorkerCallsNoTrustedMethod: an import rule cannot stop the worker from calling an exported
// method of a package it may import. The worker imports gitops, whose raw runner trustedgit builds
// push, rebase and revert on; this test type-checks every package the worker roots reach and fails
// on a call of a method in trustedOnly (ADR 0026, docs/plan/architecture/ item A7).
func TestWorkerCallsNoTrustedMethod(t *testing.T) {
	t.Parallel()
	roots := make([]string, 0, len(workerRoots))
	for _, r := range workerRoots {
		roots = append(roots, module+"internal/"+r)
	}
	pkgs := loadTyped(t, roots...)
	checked := 0
	for _, p := range pkgs {
		if !p.inModule || p.ImportPath == module+trustedOwner {
			continue
		}
		checked++
		for _, call := range trustedCalls(t, p, pkgs) {
			t.Errorf("worker package %s calls %s, which only control may call", strings.TrimPrefix(p.ImportPath, module), call)
		}
	}
	if checked < len(workerRoots) {
		t.Fatalf("type-checked %d worker packages, fewer than the %d roots; the check is broken", checked, len(workerRoots))
	}
}

// TestTrustedCallsAreFound proves the detector on the real caller: trustedgit reaches the raw
// runner through the embedded gitops.Git, so a broken detector cannot pass the worker silently.
func TestTrustedCallsAreFound(t *testing.T) {
	t.Parallel()
	path := module + "internal/control/trustedgit"
	pkgs := loadTyped(t, path)
	calls := trustedCalls(t, pkgs[path], pkgs)
	for _, want := range trustedOnly {
		if !slices.Contains(calls, want) {
			t.Errorf("trustedgit calls %s, but the detector found only %v", want, calls)
		}
	}
}

// typed is a package of go list with what type-checking it needs.
type typed struct {
	ImportPath, Dir, Export string
	GoFiles                 []string
	Module                  *struct{ Path string }
	inModule                bool
}

// loadTyped lists roots and their dependencies with export data, keyed by import path.
func loadTyped(t *testing.T, roots ...string) map[string]*typed {
	t.Helper()
	args := append([]string{"list", "-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles,Module"}, roots...)
	cmd := exec.CommandContext(t.Context(), "go", args...) //nolint:gosec // G204: the arguments are the fixed roots of the policy
	cmd.Dir = "../../.."
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			t.Fatalf("go list: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	pkgs := map[string]*typed{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p typed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		p.inModule = p.Module != nil && p.Module.Path == strings.TrimSuffix(module, "/")
		pkgs[p.ImportPath] = &p
	}
	return pkgs
}

// trustedCalls type-checks p from source against the export data of its dependencies and
// returns each method of trustedOnly that it uses, by its full name.
func trustedCalls(t *testing.T, p *typed, pkgs map[string]*typed) []string {
	t.Helper()
	if p == nil {
		t.Fatal("package not listed")
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(p.GoFiles))
	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if dep := pkgs[path]; dep != nil && dep.Export != "" {
			return os.Open(dep.Export)
		}
		return nil, fmt.Errorf("no export data for %s", path)
	}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	if _, err := conf.Check(p.ImportPath, fset, files, info); err != nil {
		t.Fatalf("type-check %s: %v", p.ImportPath, err)
	}
	var calls []string
	for _, obj := range info.Uses {
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		name := strings.Replace(fn.FullName(), module, "", 1)
		if slices.Contains(trustedOnly, name) && !slices.Contains(calls, name) {
			calls = append(calls, name)
		}
	}
	slices.Sort(calls)
	return calls
}
