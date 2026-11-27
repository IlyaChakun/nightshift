package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// designService is the part of a compose service that the design example must match.
type designService struct {
	Secrets  []designSecret `yaml:"secrets"`
	Networks []string       `yaml:"networks"`
}

// designSecret is a service secret in the short form (a name) or the long one (source, target).
type designSecret string

func (s *designSecret) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		return n.Decode((*string)(s))
	}
	var long struct{ Source string }
	if err := n.Decode(&long); err != nil {
		return err
	}
	*s = designSecret(long.Source)
	return nil
}

type designCompose struct {
	Services map[string]designService `yaml:"services"`
	Networks map[string]any           `yaml:"networks"`
	Secrets  map[string]any           `yaml:"secrets"`
	Volumes  map[string]any           `yaml:"volumes"`
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // files of this repository
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// readDesign returns the whole design: docs/design.md, then each layer file of docs/design/ in
// name order, as scripts/design-sync.sh reads it.
func readDesign(t *testing.T) string {
	t.Helper()
	text := readRepoFile(t, "../../docs/design.md")
	layers, err := filepath.Glob("../../docs/design/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range layers {
		text += "\n" + readRepoFile(t, f)
	}
	return text
}

func diffNames(t *testing.T, label string, design, actual map[string]any) {
	t.Helper()
	for k := range design {
		if _, ok := actual[k]; !ok {
			t.Errorf("%s %q is in docs/design.md but not in deploy/docker-compose.yml", label, k)
		}
	}
	for k := range actual {
		if _, ok := design[k]; !ok {
			t.Errorf("%s %q is in deploy/docker-compose.yml but not in docs/design.md", label, k)
		}
	}
}

// TestDesignComposeExample keeps the compose example of the design (section 3) in step with
// deploy/docker-compose.yml: the same services, networks, volumes and secrets, and for each
// service the same secrets and networks. Images and commands are not compared; the example
// writes versions as <version> and may abridge environment and volumes.
func TestDesignComposeExample(t *testing.T) {
	t.Parallel()
	_, rest, ok := strings.Cut(readDesign(t), "# deploy/docker-compose.yml\n")
	if !ok {
		t.Fatal("the design has no compose example that starts with '# deploy/docker-compose.yml'")
	}
	block, _, _ := strings.Cut(rest, "\n```")
	var design, actual designCompose
	if err := yaml.Unmarshal([]byte(block), &design); err != nil {
		t.Fatalf("compose example in the design: %v", err)
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, "../../deploy/docker-compose.yml")), &actual); err != nil {
		t.Fatal(err)
	}
	services := func(m map[string]designService) map[string]any {
		out := map[string]any{}
		for k := range m {
			out[k] = nil
		}
		return out
	}
	diffNames(t, "service", services(design.Services), services(actual.Services))
	diffNames(t, "network", design.Networks, actual.Networks)
	diffNames(t, "secret", design.Secrets, actual.Secrets)
	diffNames(t, "volume", design.Volumes, actual.Volumes)
	secrets := func(s designService) []string {
		out := make([]string, 0, len(s.Secrets))
		for _, r := range s.Secrets {
			out = append(out, string(r))
		}
		slices.Sort(out)
		return out
	}
	for name, d := range design.Services {
		r, ok := actual.Services[name]
		if !ok {
			continue
		}
		if a, b := secrets(d), secrets(r); !slices.Equal(a, b) {
			t.Errorf("service %q secrets: design %v, compose %v", name, a, b)
		}
		a, b := slices.Sorted(slices.Values(d.Networks)), slices.Sorted(slices.Values(r.Networks))
		if !slices.Equal(a, b) {
			t.Errorf("service %q networks: design %v, compose %v", name, a, b)
		}
	}
}

// TestDesignSubcommands keeps the list of subcommands in design section 5 equal to the commands
// the binary has.
func TestDesignSubcommands(t *testing.T) {
	t.Parallel()
	line := regexp.MustCompile("One `nightshift` binary with the subcommands ([^|]*)\\|")
	m := line.FindStringSubmatch(readDesign(t))
	if m == nil {
		t.Fatal("design section 5 has no line 'One `nightshift` binary with the subcommands ...'")
	}
	var design []string
	for _, c := range regexp.MustCompile("`([a-z]+)`").FindAllStringSubmatch(m[1], -1) {
		design = append(design, c[1])
	}
	var actual []string
	for _, c := range commands {
		actual = append(actual, c.name)
	}
	slices.Sort(design)
	slices.Sort(actual)
	if !slices.Equal(design, actual) {
		t.Errorf("subcommands: design %v, binary %v", design, actual)
	}
}

// designSync runs scripts/design-sync.sh in a copy of the repository's design files, with the
// given page in place of docs/design.html.
func designSync(t *testing.T, page string, args ...string) (string, error) {
	t.Helper()
	files := map[string]string{
		"docs/design.md":   readRepoFile(t, "../../docs/design.md"),
		"docs/design.html": page,
	}
	layers, err := filepath.Glob("../../docs/design/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range layers {
		files["docs/design/"+filepath.Base(f)] = readRepoFile(t, f)
	}
	return designSyncFiles(t, files, args...)
}

// cutSection splits text into section n of the design and the text without it. Like
// scripts/design-sync.sh, a section runs from its "## n. " heading to the next "# " or numbered
// "## N. " heading outside a code fence.
func cutSection(t *testing.T, text string, n int) (rest, section string) {
	t.Helper()
	numbered := regexp.MustCompile(`^## [0-9]+\. `)
	lines := strings.SplitAfter(text, "\n")
	start, end, fence := -1, len(lines), false
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			fence = !fence
		}
		if fence {
			continue
		}
		switch {
		case start < 0 && strings.HasPrefix(l, fmt.Sprintf("## %d. ", n)):
			start = i
		case start >= 0 && (strings.HasPrefix(l, "# ") || numbered.MatchString(l)):
			end = i
		}
		if end < len(lines) {
			break
		}
	}
	if start < 0 {
		t.Fatalf("the design has no section %d", n)
	}
	return strings.Join(lines[:start], "") + strings.Join(lines[end:], ""), strings.Join(lines[start:end], "")
}

// designSyncFiles runs scripts/design-sync.sh in a directory that holds only the given files.
func designSyncFiles(t *testing.T, files map[string]string, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	files["scripts/design-sync.sh"] = readRepoFile(t, "../../scripts/design-sync.sh")
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{filepath.Join(dir, "scripts/design-sync.sh")}, args...)...) //nolint:gosec // the repository's own script
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestDesignSyncRefusesABrokenManifest runs scripts/design-sync.sh on copies of the page whose
// manifest carries what a bad merge leaves behind: a conflict marker, or a section twice.
func TestDesignSyncRefusesABrokenManifest(t *testing.T) {
	t.Parallel()
	page := readRepoFile(t, "../../docs/design.html")
	first := regexp.MustCompile(`(?m)^§3 .*$`).FindString(page)
	if first == "" {
		t.Fatal("docs/design.html has no §3 line in its manifest")
	}
	tests := []struct {
		name, page, want string
		ok               bool
	}{
		{name: "as committed", page: page, ok: true},
		{name: "conflict marker", page: strings.Replace(page, first, first+"\n>>>>>>> theirs\n=======", 1), want: "not sections"},
		{name: "section twice", page: strings.Replace(page, first, first+"\n"+first, 1), want: "twice"},
	}
	// a section newly marked covered carries a placeholder hash until --update fills it in
	placeholder := regexp.MustCompile(`(?m)^§5 skipped`).ReplaceAllString(page, "§5 covered 0")
	if out, err := designSync(t, placeholder, "--update"); err != nil {
		t.Errorf("--update refused a placeholder hash: %v\n%s", err, out)
	}
	for _, tt := range tests {
		for _, mode := range []string{"", "--update"} {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				t.Parallel()
				var args []string
				if mode != "" {
					args = []string{mode}
				}
				out, err := designSync(t, tt.page, args...)
				if tt.ok != (err == nil) {
					t.Fatalf("err = %v, output:\n%s", err, out)
				}
				if !strings.Contains(out, tt.want) {
					t.Errorf("output lacks %q:\n%s", tt.want, out)
				}
			})
		}
	}
}

// TestDesignSyncReadsLayerFiles moves a covered section from docs/design.md into a layer file of
// docs/design/: its recorded hash still matches, and an edit there is still caught.
func TestDesignSyncReadsLayerFiles(t *testing.T) {
	t.Parallel()
	design := readDesign(t)
	page := readRepoFile(t, "../../docs/design.html")
	overview, layer := cutSection(t, design, 17)
	out, err := designSyncFiles(t, map[string]string{"docs/design.md": overview, "docs/design/tenants.md": layer, "docs/design.html": page})
	if err != nil {
		t.Fatalf("section 17 in a layer file: err = %v, output:\n%s", err, out)
	}
	// Blank lines at the end of a section do not change its hash.
	trimmed := strings.TrimRight(layer, "\n") + "\n"
	out, err = designSyncFiles(t, map[string]string{"docs/design.md": overview, "docs/design/tenants.md": trimmed + "\n\n", "docs/design.html": page})
	if err != nil {
		t.Fatalf("section 17 with trailing blank lines: err = %v, output:\n%s", err, out)
	}
	out, err = designSyncFiles(t, map[string]string{"docs/design.md": overview, "docs/design/tenants.md": trimmed, "docs/design.html": page})
	if err != nil {
		t.Fatalf("section 17 without trailing blank lines: err = %v, output:\n%s", err, out)
	}
	edited := strings.Replace(layer, "### Four layers", "### Four layers\n\nAn edit.", 1)
	if edited == layer {
		t.Fatal("section 17 has no heading \"### Four layers\" to edit")
	}
	out, err = designSyncFiles(t, map[string]string{"docs/design.md": overview, "docs/design/tenants.md": edited, "docs/design.html": page})
	if err == nil || !strings.Contains(out, "§17 of the design changed") {
		t.Errorf("an edit of section 17 in a layer file passed: err = %v, output:\n%s", err, out)
	}
	// A layer file's own title and intro belong to no section, whichever file comes first.
	rest, section18 := cutSection(t, overview, 18)
	notes := "# Notes layer\n\nIntro of the layer.\n\n" + section18
	out, err = designSyncFiles(t, map[string]string{"docs/design.md": rest, "docs/design/a-tenants.md": layer, "docs/design/b-notes.md": notes, "docs/design.html": page})
	if err != nil {
		t.Errorf("a layer file with a title: err = %v, output:\n%s", err, out)
	}
	// A section in two files is refused.
	out, err = designSyncFiles(t, map[string]string{"docs/design.md": design, "docs/design/tenants.md": layer, "docs/design.html": page})
	if err == nil || !strings.Contains(out, "two places: 17") {
		t.Errorf("section 17 in two files passed: err = %v, output:\n%s", err, out)
	}
}
