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

	"go.yaml.in/yaml/v3"
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
	vendors, _ := depguardCore(t)
	if len(vendors) == 0 {
		t.Fatal("found no vendor SDKs in the depguard rule of .golangci.yml")
	}
	mod := map[string]pkg{}
	for _, p := range pkgs {
		mod[p.Path] = p
	}
	msgs := append(enforce(pkgs, exceptions, vendors), missingRoots(workerRoots, mod)...)
	msgs = append(msgs, reachDispatch(leftDispatch, mod)...)
	for _, msg := range msgs {
		t.Error(msg)
	}
}

// listModule reads the production import graph with go list, from the module root.
func listModule(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json=ImportPath,Imports", "./...")
	cmd.Dir = "../../.."
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
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
			unknown = append(unknown, fmt.Sprintf("%s has no plane; add it to planes in internal/testing/archtest/policy_test.go", p.Path))
			continue
		}
		if msg := folderProblem(p.Path, from); msg != "" {
			unknown = append(unknown, msg)
		}
		for _, imp := range p.Imports {
			vs = append(vs, edgeViolations(p.Path, from, imp, inModule, vendors)...)
		}
	}
	return append(vs, workerViolations(inModule)...), unknown
}

// edgeViolations checks one production import of a package of plane from.
func edgeViolations(path string, from plane, imp string, inModule map[string]pkg, vendors []string) []violation {
	var vs []violation
	if v, ok := vendorViolation(path, imp, vendors); ok {
		vs = append(vs, v)
	}
	if _, ok := inModule[imp]; !ok {
		return vs
	}
	to, ok := planeOf(imp)
	switch {
	case !ok:
		// reported once as unknown when the loop reaches imp
	case concreteAdapter(imp) && from != root && from != testPlane:
		vs = append(vs, violation{path, imp, "only a composition root imports a concrete adapter", string(from)})
	case !mayImport(from, to, imp):
		vs = append(vs, violation{path, imp, "plane may not import plane", string(from) + " → " + string(to)})
	}
	return vs
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

// depguardCore reads the depguard rule "core" of .golangci.yml: the denied vendor SDKs and the
// package trees its file exclusions ("!**/<tree>/**") leave free to use them.
func depguardCore(t *testing.T) (denied, owners []string) {
	t.Helper()
	raw, err := os.ReadFile("../../../.golangci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Linters struct {
			Settings struct {
				Depguard struct {
					Rules map[string]struct {
						Files []string
						Deny  []struct{ Pkg string }
					}
				}
			}
		}
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .golangci.yml: %v", err)
	}
	core, ok := cfg.Linters.Settings.Depguard.Rules["core"]
	if !ok {
		t.Fatal(".golangci.yml has no depguard rule core")
	}
	for _, d := range core.Deny {
		denied = append(denied, d.Pkg)
	}
	for _, f := range core.Files {
		if tree, ok := strings.CutPrefix(f, "!**/"); ok {
			owners = append(owners, strings.TrimSuffix(tree, "/**"))
		}
	}
	return denied, owners
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

// folderProblem reports a package in a plane folder that planes puts in another plane. A twin
// (a package ending in /fake) or a contract suite (harnesstest, deploytest) may sit
// beside its port and stay in the testing plane, so production code of the folder's plane still
// cannot import it.
// contractSuites are the contract suites that sit beside their port in a plane folder.
var contractSuites = []string{"internal/execution/harness/harnesstest", "internal/execution/deployapi/deploytest"}

func folderProblem(path string, from plane) string {
	if from == testPlane && (strings.HasSuffix(path, "/fake") || slices.Contains(contractSuites, path)) {
		return ""
	}
	if want, ok := folderPlane(path); ok && want != from {
		return fmt.Sprintf("%s is in the %s folder but classified as %s; move it or fix planes", path, folders[want], from)
	}
	return ""
}

// folderPlane returns the plane whose folder holds path, if path is under internal/<plane folder>/.
func folderPlane(path string) (plane, bool) {
	rest, ok := strings.CutPrefix(path, "internal/")
	if !ok {
		return "", false
	}
	top, _, _ := strings.Cut(rest, "/")
	for p, f := range folders {
		if f == top {
			return p, true
		}
	}
	return "", false
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

// missingRoots names each worker root and book prefix that is no longer a package of the module, so a move
// cannot turn the worker-to-book check into one that checks nothing.
func missingRoots(roots []string, mod map[string]pkg) []string {
	var out []string
	for _, name := range roots {
		if mod["internal/"+name].Path == "" {
			out = append(out, fmt.Sprintf("worker root internal/%s is not a package; fix workerRoots in internal/testing/archtest/policy_test.go", name))
		}
	}
	for _, prefix := range bookOnly {
		if strings.HasPrefix(prefix, "internal/") && mod[prefix].Path == "" {
			out = append(out, fmt.Sprintf("book package %s is not a package; fix bookOnly in internal/testing/archtest/policy_test.go", prefix))
		}
	}
	return out
}

// workerViolations reports every import of the book or a Postgres client by a package the worker
// roots reach, with the path that reaches it. Each importing edge is its own violation, so an
// exception excuses one edge and a new route to the book fails.
func workerViolations(mod map[string]pkg) []violation {
	prev := map[string]string{}
	var queue []string
	for _, name := range workerRoots {
		if r := "internal/" + name; mod[r].Path != "" {
			prev[r] = ""
			queue = append(queue, r)
		}
	}
	var vs []violation
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, imp := range mod[cur].Imports {
			if bookPackage(imp) {
				vs = append(vs, violation{cur, imp, "the worker side reaches the book", strings.Join(pathTo(prev, cur), " → ")})
				continue
			}
			if _, seen := prev[imp]; seen {
				continue
			}
			if _, ok := mod[imp]; ok { // packages outside the module are not followed
				prev[imp] = cur
				queue = append(queue, imp)
			}
		}
	}
	return vs
}

// reachDispatch reports each package of from that does not exist or reaches the conductor's package,
// with the import path that reaches it.
func reachDispatch(from []string, mod map[string]pkg) []string {
	const target = "internal/control/dispatch"
	var out []string
	for _, root := range from {
		if mod[root].Path == "" {
			out = append(out, fmt.Sprintf("%s is not a package; fix leftDispatch in internal/testing/archtest/policy_test.go", root))
			continue
		}
		prev := map[string]string{root: ""}
		queue := []string{root}
		for len(queue) > 0 && prev[target] == "" {
			cur := queue[0]
			queue = queue[1:]
			for _, imp := range mod[cur].Imports {
				if _, seen := prev[imp]; seen {
					continue
				}
				if _, ok := mod[imp]; ok {
					prev[imp] = cur
					queue = append(queue, imp)
				}
			}
		}
		if prev[target] != "" {
			out = append(out, fmt.Sprintf("%s reaches %s (ADR 0027): %s", root, target, strings.Join(pathTo(prev, target), " → ")))
		}
	}
	return out
}

func bookPackage(path string) bool {
	return slices.ContainsFunc(bookOnly, func(p string) bool { return path == p || strings.HasPrefix(path, p+"/") })
}

func pathTo(prev map[string]string, n string) []string {
	var path []string
	for ; n != ""; n = prev[n] {
		path = append([]string{n}, path...)
	}
	return path
}
