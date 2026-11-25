package stack_test

import (
	"errors"
	"os"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/stack"
)

func TestTheShippedPacksLoadAndDetectTheirStacks(t *testing.T) {
	t.Parallel()
	packs, err := stack.Load(os.DirFS("../../stacks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go", "python", "ts", "js", "java-maven", "java-gradle"} {
		if _, ok := packs[name]; !ok {
			t.Errorf("no pack %s", name)
		}
	}
	repo := fstest.MapFS{"go.mod": {Data: []byte("module x")}, "tsconfig.json": {Data: []byte("{}")}}
	if got := packs.Detect(repo); !slices.Equal(got, []string{"go", "ts"}) {
		t.Errorf("detected %v", got)
	}
}

func TestLoadRejectsBadPacks(t *testing.T) {
	t.Parallel()
	for name, fsys := range map[string]fstest.MapFS{
		"wrong name":   {"go/pack.yaml": {Data: []byte("name: rust\ndetect: [x]\ntoolchain: [cargo]\ntools: []\n")}},
		"no tools":     {"go/pack.yaml": {Data: []byte("name: go\ndetect: [go.mod]\ntoolchain: [go]\n")}},
		"bad tool":     {"go/pack.yaml": {Data: []byte("name: go\ndetect: [go.mod]\ntoolchain: [go]\ntools: [{name: x, command: y, category: vibes}]\n")}},
		"no pack.yaml": {"go/readme.md": {Data: []byte("x")}},
	} {
		if _, err := stack.Load(fsys); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestMissingAndToolsReportWhatTheWorkerLacks(t *testing.T) {
	t.Parallel()
	packs, err := stack.Load(os.DirFS("../../stacks"))
	if err != nil {
		t.Fatal(err)
	}
	look := func(bin string) error {
		if bin == "node" {
			return errors.New("not found")
		}
		return nil
	}
	if got := packs.Missing([]string{"go", "ts"}, look); !slices.Equal(got, []string{"node"}) {
		t.Errorf("missing %v", got)
	}
	// A tool whose executable is absent is skipped and named, never run.
	tools, skipped := packs.Tools([]string{"python"})
	for _, tool := range tools {
		if tool.Command == "" {
			t.Errorf("empty command: %+v", tool)
		}
	}
	if len(tools)+len(skipped) != 2 {
		t.Errorf("python has two tools, got %d run and %v skipped", len(tools), skipped)
	}
}

func TestDetectUnlessKeepsATypeScriptProjectOutOfJS(t *testing.T) {
	t.Parallel()
	packs, err := stack.Load(os.DirFS("../../stacks"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"plain js":   {"package.json"},
		"ts":         {"package.json", "tsconfig.json"},
		"maven":      {"pom.xml"},
		"gradle kts": {"build.gradle.kts", "settings.gradle.kts"},
		"go and js":  {"go.mod", "package.json"},
	}
	want := map[string][]string{
		"plain js": {"js"}, "ts": {"ts"}, "maven": {"java-maven"}, "gradle kts": {"java-gradle"}, "go and js": {"go", "js"},
	}
	for name, files := range cases {
		repo := fstest.MapFS{}
		for _, f := range files {
			repo[f] = &fstest.MapFile{Data: []byte("x")}
		}
		if got := packs.Detect(repo); !slices.Equal(got, want[name]) {
			t.Errorf("%s: detected %v, want %v", name, got, want[name])
		}
	}
}
