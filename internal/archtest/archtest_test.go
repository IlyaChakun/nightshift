// Package archtest_test enforces the plane boundaries of ADR 0026 on the real import graph:
// every package has a plane, production imports follow the allowed planes, only composition roots
// import concrete adapters, and the worker side never reaches the book. Exceptions are exact and
// must still match a violation (design §26, docs/plan/architecture/ item A2).
package archtest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/ilyachakun/nightshift/"

// pkg is one production package of the module: its path relative to the module and its
// production imports (module packages relative, others as they are).
type pkg struct {
	Path    string
	Imports []string
}

type exception struct {
	From, To, Item, Why string
}

// violation is one broken rule, keyed by its edge so an exception can match it.
type violation struct {
	From, To, Rule, Detail string
}

func (v violation) String() string {
	return fmt.Sprintf("%s → %s: %s (%s)", v.From, v.To, v.Rule, v.Detail)
}

// TestPlaneBoundaries runs the policy on the module's real graph.
func TestPlaneBoundaries(t *testing.T) {
	t.Parallel()
	pkgs := listModule(t)
	if len(pkgs) < 10 {
		t.Fatalf("go list found %d packages; the graph is incomplete", len(pkgs))
	}
	vendors := depguardDenied(t)
	if len(vendors) == 0 {
		t.Fatal("found no vendor SDKs in the depguard rule of .golangci.yml")
	}
	for _, msg := range enforce(pkgs, exceptions, vendors) {
		t.Error(msg)
	}
}

// listModule reads the production import graph with go list, from the module root.
func listModule(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=ImportPath,Imports", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("go list: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	var pkgs []pkg
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		q := pkg{Path: strings.TrimPrefix(p.ImportPath, module)}
		for _, imp := range p.Imports {
			q.Imports = append(q.Imports, strings.TrimPrefix(imp, module))
		}
		pkgs = append(pkgs, q)
	}
	return pkgs
}

// enforce returns one message per violation without a matching exception and per exception
// that matches nothing.
func enforce(pkgs []pkg, excs []exception, vendors []string) []string {
	vs, msgs := violations(pkgs, vendors)
	used := make([]bool, len(excs))
	for _, v := range vs {
		i := slices.IndexFunc(excs, func(e exception) bool { return e.From == v.From && e.To == v.To })
		if i >= 0 {
			used[i] = true
			continue
		}
		msgs = append(msgs, v.String())
	}
	for i, e := range excs {
		if !used[i] {
			msgs = append(msgs, fmt.Sprintf("exception %s → %s (%s, %s) matches no violation; delete it", e.From, e.To, e.Item, e.Why))
		}
	}
	return msgs
}

// violations checks every production edge of the module and the worker's reach. The second
// result lists packages without a plane, which no exception can excuse.
func violations(pkgs []pkg, vendors []string) ([]violation, []string) {
	var vs []violation
	var unknown []string
	inModule := map[string]pkg{}
	for _, p := range pkgs {
		inModule[p.Path] = p
	}
	for _, p := range pkgs {
		from, ok := planeOf(p.Path)
		if !ok {
			unknown = append(unknown, fmt.Sprintf("%s has no plane; add it to planes in internal/archtest/policy_test.go", p.Path))
			continue
		}
		for _, imp := range p.Imports {
			if v, ok := vendorViolation(p.Path, imp, vendors); ok {
				vs = append(vs, v)
			}
			if _, ok := inModule[imp]; !ok {
				continue
			}
			to, ok := planeOf(imp)
			if !ok {
				continue // reported once as unknown above
			}
			if concreteAdapter(imp) && from != root && from != testPlane {
				vs = append(vs, violation{p.Path, imp, "only a composition root imports a concrete adapter", string(from)})
				continue
			}
			if !mayImport(from, to, imp) {
				vs = append(vs, violation{p.Path, imp, "plane may not import plane", string(from) + " → " + string(to)})
			}
		}
	}
	if _, ok := inModule[workerRoot]; ok {
		for _, prefix := range bookOnly {
			if path := reach(inModule, workerRoot, prefix); path != nil {
				vs = append(vs, violation{workerRoot, prefix, "the worker side reaches the book", strings.Join(path, " → ")})
			}
		}
	}
	return vs, unknown
}

// vendorViolation reports an import of a vendor SDK outside the package trees that own them.
func vendorViolation(from, imp string, vendors []string) (violation, bool) {
	for _, prefix := range vendors {
		if imp != prefix && !strings.HasPrefix(imp, prefix+"/") {
			continue
		}
		for _, owner := range vendorOwners {
			if from == owner || strings.HasPrefix(from, owner+"/") {
				return violation{}, false
			}
		}
		return violation{from, imp, "vendor SDK outside its owners", strings.Join(vendorOwners, ", ")}, true
	}
	return violation{}, false
}

// depguardDenied reads the "- pkg:" lines of .golangci.yml: the vendor SDKs the depguard rule
// "core" keeps out of the core.
func depguardDenied(t *testing.T) []string {
	t.Helper()
	lint, err := os.ReadFile("../../.golangci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var denied []string
	for _, line := range strings.Split(string(lint), "\n") {
		if pkg, ok := strings.CutPrefix(strings.TrimSpace(line), "- pkg: "); ok {
			denied = append(denied, pkg)
		}
	}
	return denied
}

func mayImport(from, to plane, target string) bool {
	if from == root || from == testPlane || from == to {
		return true
	}
	if !slices.Contains(allowed[from], to) {
		return false
	}
	if only, ok := ports[[2]plane{from, to}]; ok {
		return slices.Contains(only, strings.TrimPrefix(target, "internal/"))
	}
	return true
}

// planeOf finds the plane of a module path: an exact entry, else the longest "/..." entry above it.
func planeOf(path string) (plane, bool) {
	if p, ok := planes[path]; ok {
		return p, true
	}
	best, found := "", plane("")
	for k, p := range planes {
		base, tree := strings.CutSuffix(k, "/...")
		if tree && (path == base || strings.HasPrefix(path, base+"/")) && len(base) > len(best) {
			best, found = base, p
		}
	}
	return found, best != ""
}

// reach returns the shortest import path from start to any package with the prefix, or nil.
func reach(mod map[string]pkg, start, prefix string) []string {
	prev := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, imp := range mod[cur].Imports {
			if _, seen := prev[imp]; seen {
				continue
			}
			prev[imp] = cur
			if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
				var path []string
				for n := imp; n != ""; n = prev[n] {
					path = append([]string{n}, path...)
				}
				return path
			}
			if _, ok := mod[imp]; ok {
				queue = append(queue, imp)
			}
		}
	}
	return nil
}
