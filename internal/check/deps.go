package check

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

var (
	shaRe     = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	reqName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*`)
	goModDep  = regexp.MustCompile(`^([A-Za-z0-9][\w.~-]*\.[\w.~-]+(?:/[\w.~-]+)*)\s+v\d`)
	requireIn = regexp.MustCompile(`^require\s*\(\s*$`)
)

// manifestDeps returns the dependency names a manifest declares, by the manifest's file name.
// Names of other files give nil.
func manifestDeps(name, text string, direct bool) map[string]bool {
	switch {
	case name == "go.mod":
		return goModDeps(text, direct)
	case name == "package.json":
		return packageJSONDeps(text)
	case strings.HasSuffix(name, ".txt") && strings.Contains(name, "requirements"):
		return requirementsDeps(text)
	}
	return nil
}

// goModDeps reads the require lines, in blocks and alone; replace, exclude and retract are not
// dependencies. A direct and an indirect requirement both count: moving one between the two
// is not a new dependency. With direct set, indirect requirements are left out: they are what
// `go mod tidy` adds for a direct one.
func goModDeps(text string, direct bool) map[string]bool {
	out := map[string]bool{}
	in := false
	for l := range strings.SplitSeq(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case requireIn.MatchString(l):
			in = true
		case l == ")":
			in = false
		case in:
			if m := goModDep.FindStringSubmatch(l); m != nil && (!direct || !strings.Contains(l, "// indirect")) {
				out[m[1]] = true
			}
		case strings.HasPrefix(l, "require "):
			if m := goModDep.FindStringSubmatch(strings.TrimSpace(strings.TrimPrefix(l, "require "))); m != nil && (!direct || !strings.Contains(l, "// indirect")) {
				out[m[1]] = true
			}
		}
	}
	return out
}

func packageJSONDeps(text string) map[string]bool {
	var p map[string]json.RawMessage
	out := map[string]bool{}
	if json.Unmarshal([]byte(text), &p) != nil {
		return out
	}
	for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		var m map[string]json.RawMessage
		if json.Unmarshal(p[section], &m) == nil {
			for k := range m {
				out[k] = true
			}
		}
	}
	return out
}

func requirementsDeps(text string) map[string]bool {
	out := map[string]bool{}
	for l := range strings.SplitSeq(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "-") {
			continue
		}
		if n := reqName.FindString(l); n != "" {
			out[strings.ToLower(n)] = true
		}
	}
	return out
}

// DependencyFindings reports every dependency the patch adds to a manifest when no ADR in
// docs/adr lists it under "Considered Options": the research record is missing (design section 6).
// The finding blocks, because a new dependency is blast radius `critical` and the owner needs
// the record to decide. base is the commit the patch starts from. Manifests are found at any depth
// by walking the checkout, and compared with their version at base. Git runs as the agent user
// when agent is set, and only reads blobs: no command here diffs the worktree, so no clean filter
// or other program from the agent's checkout configuration is started.
func DependencyFindings(ctx context.Context, dir, base string, agent *Agent) ([]Finding, error) {
	if base == "" {
		return nil, nil
	}
	if !shaRe.MatchString(base) {
		return nil, fmt.Errorf("check: dependencies: base %q is not a commit id", base)
	}
	records := adrOptions(dir)
	var out []Finding
	// The id must name a commit object, not a branch, tag or replacement the agent made.
	if _, err := git(ctx, agent, dir, "cat-file", "-e", base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("check: dependencies: base %s is not a commit of the checkout", base)
	}
	manifests, links, err := findManifests(dir)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		out = append(out, linkFinding(l))
	}
	for _, rel := range manifests {
		name := path.Base(rel)
		head, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))) //nolint:gosec // G304: a regular file found by findManifests inside the worktree
		if err != nil {
			continue
		}
		old, _ := git(ctx, agent, dir, "show", base+":"+rel) // a manifest the patch adds has no base version
		before := manifestDeps(name, old, false)
		for dep := range manifestDeps(name, string(head), true) {
			if !before[dep] && !named(records, dep) {
				out = append(out, depFinding(rel, dep))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Location.Path != out[j].Location.Path {
			return out[i].Location.Path < out[j].Location.Path
		}
		return out[i].Message < out[j].Message
	})
	return out, nil
}

// linkFinding blocks a manifest that is a link: Go and npm follow it, and its target is not read here.
func linkFinding(p string) Finding {
	f := Finding{
		RuleID: "dependency.manifest-link", Category: "process", Severity: Block, LayerCaught: "L2",
		Message: p + " is a link, so the dependencies it declares cannot be checked; make it a regular file", Location: &Location{Path: p},
	}
	f.Fingerprint = fingerprint(f, nil)
	return f
}

func depFinding(p, name string) Finding {
	f := Finding{
		RuleID: "dependency.no-research-record", Category: "process", Severity: Block, LayerCaught: "L2",
		Message: "the patch adds the dependency " + name + " and no ADR in docs/adr lists it under \"Considered Options\"; stop with needs_research, " +
			"so Lead records the choice (build-vs-buy, options including writing it ourselves)",
		Location: &Location{Path: p},
	}
	f.Fingerprint = fingerprint(f, nil)
	return f
}

// adrOptions is the "Considered Options" text of every ADR of the worktree.
func adrOptions(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "docs", "adr", "*.md"))
	var b strings.Builder
	for _, f := range files {
		if raw, err := os.ReadFile(f); err == nil { //nolint:gosec // G304: a glob of the worktree's ADRs
			b.WriteString(section(string(raw), "Considered Options"))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// named reports whether text mentions the package as a whole word, so `example.com/new` is not
// found in `example.com/newer`.
func named(text, name string) bool {
	re, err := regexp.Compile(`(?i)(^|[^\w./@-])` + regexp.QuoteMeta(name) + `($|[^\w@-]|[./-]\s|[./-]$)`)
	return err == nil && re.MatchString(text)
}

// skipDirs are not walked: they hold no project manifest the patch could add.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true}

// findManifests lists the dependency manifests of the checkout, as slash paths, and separately
// the manifests that are links or special files, which are never read.
func findManifests(dir string) (files, links []string, err error) {
	err = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if manifestDeps(d.Name(), "", false) == nil {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, filepath.ToSlash(rel))
		} else {
			links = append(links, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("check: dependencies: %w", err)
	}
	return files, links, nil
}

// git runs one read-only git command that reads a blob. The checkout belongs to the agent, so
// its .git/config and .gitattributes are not trusted: the command runs as the agent user when
// one is set (a program it starts has no more rights than the agent has), with the global and
// system configuration off.
func git(ctx context.Context, agent *Agent, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "safe.directory=*", "-c", "protocol.allow=never"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) //nolint:gosec // G204: fixed program; the arguments are built here and the base is checked to be a commit id
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	// No replacement objects, no lazy fetch and no transport: the agent's .git/config must not
	// change what base means or start a network command or a program.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", //nolint:forbidigo // git needs the PATH, nothing else
		"GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0"}
	if agent != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: agent.UID, Gid: agent.GID}}
		cmd.Env[1] = "HOME=" + agent.Home
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("check: git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}
