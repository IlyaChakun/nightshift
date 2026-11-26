package composelocal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/integration"
)

func TestLintSourceRefusesAnotherComposeFile(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, detail string }{
		{"include, short", "include:\n  - other.yaml\nservices:\n  web:\n    image: x\n", "include"},
		{"include, long", "include:\n  - path: other.yaml\nservices: {web: {image: x}}\n", "include"},
		{"include, quoted key", "\"include\": [other.yaml]\nservices: {web: {image: x}}\n", "include"},
		{"include in a second document", "services: {web: {image: x}}\n---\ninclude: [other.yaml]\n", "include"},
		{"merge at the top level", "x-a: &a\n  include: [other.yaml]\n<<: *a\nservices: {web: {image: x}}\n", "a merge key at the top level"},
		{"extends a file", "services:\n  web:\n    extends:\n      file: other.yaml\n      service: web\n", "extends"},
		{"extends a file, flow", "services: {web: {extends: {file: other.yaml, service: web}}}\n", "extends"},
		{"extends by an alias", "x-e: &e {file: other.yaml, service: web}\nservices:\n  web:\n    extends: *e\n", "extends"},
		{"extends inside an anchor", "x-s: &s\n  extends: {file: other.yaml, service: web}\nservices:\n  web:\n    <<: *s\n", "extends"},
		{"extends with a merged file", "x-e: &e {file: other.yaml}\nservices:\n  web:\n    extends:\n      <<: *e\n      service: web\n", "extends"},
		{"extends a list", "services: {web: {extends: [other.yaml]}}\n", "extends"},
	} {
		got := LintSource([]byte(c.src))
		found := false
		for _, v := range got {
			found = found || (v.Rule == "another compose file" && v.Detail == c.detail)
		}
		if !found {
			t.Errorf("%s: violations = %v, want %q", c.name, got, c.detail)
		}
	}
}

func TestLintSourcePassesOneFile(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"plain":                  "services:\n  web:\n    image: x\n    environment:\n      include: \"1\"\n      extends: no\n",
		"extends a sibling":      "services:\n  base: {image: x}\n  web:\n    extends: base\n  api:\n    extends:\n      service: base\n",
		"anchors and merge keys": "x-common: &c\n  restart: always\nservices:\n  web:\n    <<: *c\n    image: x\n",
	} {
		if got := LintSource([]byte(src)); len(got) != 0 {
			t.Errorf("%s: violations = %v, want none", name, got)
		}
	}
}

func TestLintSourceNeverPassesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"empty":                        "",
		"not YAML":                     "services: [unclosed\n",
		"a list":                       "- services\n",
		"a scalar":                     "services\n",
		"a list after a good document": "services: {web: {image: x}}\n---\n- a\n",
	} {
		if got := LintSource([]byte(src)); len(got) == 0 {
			t.Errorf("%s passed", name)
		}
	}
}

// A file of aliases that expands to a tree of 2^40 nodes is walked node by node, not expanded.
func TestLintSourceWalksEachNodeOnce(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("x-a0: &a0 {image: x}\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "x-a%d: &a%d {l: *a%d, r: *a%d}\n", i, i, i-1, i-1)
	}
	b.WriteString("services: {web: {image: x}}\n")
	if got := LintSource([]byte(b.String())); len(got) != 0 {
		t.Fatalf("violations = %v, want none", got)
	}
}

// Up must refuse the recipe file before the engine's CLI is asked to render it: the render is
// what reads the other file.
func TestUpRefusesAnotherComposeFileBeforeTheRender(t *testing.T) {
	t.Parallel()
	for name, recipe := range map[string]string{
		"include": "include: [other.yaml]\nservices: {web: {image: x}}\n",
		"extends": "services: {web: {extends: {file: other.yaml, service: web}}}\n",
	} {
		f := &fakeExec{rendered: okRecipe}
		_, err := deployerIn(t, f, recipe).Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "t1"}, "abc")
		if !errors.Is(err, integration.ErrRefused) {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: the engine was called: %v", name, f.calls)
		}
	}
}

// The checkout is the change's: a `.env` or a recipe that is a link out of it is not read.
func TestUpRefusesAFileThatLeavesTheCheckoutByALink(t *testing.T) {
	t.Parallel()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("services: {web: {image: x}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", "compose.yaml"} {
		f := &fakeExec{rendered: okRecipe}
		d := deployerIn(t, f, "services: {web: {image: x}}\n")
		dir := d.Checkouts.(tree).dir
		link := filepath.Join(dir, name)
		_ = os.Remove(link)
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		_, err := d.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "t1"}, "abc")
		if !errors.Is(err, integration.ErrRefused) || !strings.Contains(err.Error(), "file outside the checkout") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: the engine was called: %v", name, f.calls)
		}
	}
	// A plain `.env` inside the checkout is the project's own and stays allowed.
	f := &fakeExec{rendered: okRecipe, psOut: map[string]string{"compose": `{"Publishers":[{"PublishedPort":8080}]}`}}
	d := deployerIn(t, f, "services: {web: {image: x}}\n")
	if err := os.WriteFile(filepath.Join(d.Checkouts.(tree).dir, ".env"), []byte("A=b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "t1"}, "abc"); err != nil {
		t.Errorf("a plain .env: %v", err)
	}
}

func TestUpWithoutARecipeFileFails(t *testing.T) {
	t.Parallel()
	f := &fakeExec{rendered: okRecipe}
	d := deployerIn(t, f, "services: {web: {image: x}}\n")
	if err := os.Remove(d.Checkouts.(tree).recipe); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "t1"}, "abc"); err == nil || len(f.calls) != 0 {
		t.Errorf("err = %v, calls = %v; want an error before any call", err, f.calls)
	}
}
