package check

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T, files map[string]string) (dir, base string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) string {
		//nolint:gosec // G204: a fixed program in a test
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	for name, body := range files {
		write(t, filepath.Join(dir, name), body)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir, run("rev-parse", "HEAD")
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestANewDependencyWithoutARecordIsAFinding(t *testing.T) {
	dir, base := repo(t, map[string]string{
		"go.mod":       "module example.com/app\n\ngo 1.27\n\nrequire example.com/old v1.0.0\n",
		"package.json": "{\n  \"version\": \"1.0.0\",\n  \"dependencies\": {\n    \"left-pad\": \"^1.0.0\"\n  }\n}\n",
	})
	write(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire (\n\texample.com/old v1.0.0\n\texample.com/new v0.3.1\n\texample.com/dep v1.0.0 // indirect\n)\n")
	write(t, filepath.Join(dir, "package.json"), "{\n  \"version\": \"1.1.0\",\n  \"dependencies\": {\n    \"left-pad\": \"^1.0.0\",\n    \"@scope/pkg\": \"^2.0.0\"\n  }\n}\n")
	write(t, filepath.Join(dir, "requirements.txt"), "requests==2.32.0\n")
	got, err := DependencyFindings(t.Context(), dir, base)
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for _, f := range got {
		if f.RuleID != "dependency.no-research-record" || f.Severity != Block {
			t.Fatalf("finding: %+v", f)
		}
		msgs = append(msgs, f.Message)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"example.com/new", "@scope/pkg"} {
		if !strings.Contains(all, want) {
			t.Errorf("no finding for %s in:\n%s", want, all)
		}
	}
	for _, not := range []string{"example.com/dep", "dependency version", "left-pad", "example.com/old"} {
		if strings.Contains(all, not) {
			t.Errorf("unexpected finding for %s", not)
		}
	}
}

func TestAnADRThatNamesTheDependencyIsTheRecord(t *testing.T) {
	dir, base := repo(t, map[string]string{"go.mod": "module example.com/app\n\ngo 1.27\n"})
	write(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire example.com/new v0.3.1\n")
	write(t, filepath.Join(dir, "docs", "adr", "0001-use-new.md"), "# Use example.com/new\n\n## Considered Options\n\n- write it ourselves\n- example.com/new\n")
	got, err := DependencyFindings(t.Context(), dir, base)
	if err != nil || len(got) != 0 {
		t.Fatalf("findings %v, err %v", got, err)
	}
	if got, _ := DependencyFindings(t.Context(), dir, ""); got != nil {
		t.Fatal("without a base the rule is off")
	}
}

func TestDependencyParsingEdgeCases(t *testing.T) {
	dir, base := repo(t, map[string]string{
		"go.mod":             "module example.com/app\n\ngo 1.27\n\nrequire example.com/was v1.0.0 // indirect\n",
		"web/package.json":   "{\"scripts\":{\"x\":\"1.0.0\"},\"engines\":{\"node\":\">=20\"}}\n",
		"requirements.txt":   "",
		"docs/adr/0001-x.md": "# X\n\n## Considered Options\n\n- example.com/newer\n- write it ourselves\n",
	})
	write(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire (\n\texample.com/was v1.0.0\n\texample.com/new v0.1.0\n)\n\nexclude example.com/bad v1.0.0\nreplace example.com/was => example.com/fork v1.0.0\n")
	write(t, filepath.Join(dir, "web", "package.json"), "{\"scripts\":{\"x\":\"1.0.0\"},\"engines\":{\"node\":\">=20\",\"yarn\":\">=1\"},\"dependencies\":{\"workspace-lib\":\"workspace:*\"}}\n")
	write(t, filepath.Join(dir, "api", "dev-requirements.txt"), "-r base.txt\n# comment\nrequests[security]==2.0 ; python_version>'3'\n")
	got, err := DependencyFindings(t.Context(), dir, base)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range got {
		names = append(names, f.Location.Path+":"+strings.Fields(strings.TrimPrefix(f.Message, "the patch adds the dependency "))[0])
	}
	want := []string{"api/dev-requirements.txt:requests", "go.mod:example.com/new", "web/package.json:workspace-lib"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", names, want)
	}
}

func TestANonCommitBaseIsRefused(t *testing.T) {
	if _, err := DependencyFindings(t.Context(), t.TempDir(), "--output=/tmp/x"); err == nil {
		t.Fatal("a base that is not a commit id must be refused")
	}
}
