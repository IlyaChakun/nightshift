package main

import (
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

// TestDesignComposeExample keeps the compose example in docs/design.md (section 3) in step with
// deploy/docker-compose.yml: the same services, networks, volumes and secrets, and for each
// service the same secrets and networks. Images and commands are not compared; the example
// writes versions as <version> and may abridge environment and volumes.
func TestDesignComposeExample(t *testing.T) {
	t.Parallel()
	_, rest, ok := strings.Cut(readRepoFile(t, "../../docs/design.md"), "# deploy/docker-compose.yml\n")
	if !ok {
		t.Fatal("docs/design.md has no compose example that starts with '# deploy/docker-compose.yml'")
	}
	block, _, _ := strings.Cut(rest, "\n```")
	var design, actual designCompose
	if err := yaml.Unmarshal([]byte(block), &design); err != nil {
		t.Fatalf("compose example in docs/design.md: %v", err)
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
	m := line.FindStringSubmatch(readRepoFile(t, "../../docs/design.md"))
	if m == nil {
		t.Fatal("docs/design.md section 5 has no line 'One `nightshift` binary with the subcommands ...'")
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, body := range map[string]string{
				"scripts/design-sync.sh": readRepoFile(t, "../../scripts/design-sync.sh"),
				"docs/design.md":         readRepoFile(t, "../../docs/design.md"),
				"docs/design.html":       tt.page,
			} {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil { //nolint:gosec // a path under t.TempDir()
					t.Fatal(err)
				}
			}
			for _, args := range [][]string{nil, {"--update"}} {
				out, err := exec.CommandContext(t.Context(), "bash", append([]string{filepath.Join(dir, "scripts/design-sync.sh")}, args...)...).CombinedOutput() //nolint:gosec // the repository's own script
				if tt.ok != (err == nil) {
					t.Fatalf("design-sync %v: err = %v, output:\n%s", args, err, out)
				}
				if !strings.Contains(string(out), tt.want) {
					t.Errorf("design-sync %v: output lacks %q:\n%s", args, tt.want, out)
				}
			}
		})
	}
}
