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
	got, err := DependencyFindings(t.Context(), dir, base, nil)
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
	got, err := DependencyFindings(t.Context(), dir, base, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("findings %v, err %v", got, err)
	}
	if got, _ := DependencyFindings(t.Context(), dir, "", nil); got != nil {
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
	got, err := DependencyFindings(t.Context(), dir, base, nil)
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
	if _, err := DependencyFindings(t.Context(), t.TempDir(), "--output=/tmp/x", nil); err == nil {
		t.Fatal("a base that is not a commit id must be refused")
	}
}

// evilCheckout is a checkout whose configuration would run a program (a clean filter, a
// fsmonitor, an external diff) in any git command that diffs or stats the worktree. The marker
// file shows whether one ran.
func evilCheckout(t *testing.T) (dir, base, marker string) {
	t.Helper()
	dir, base = repo(t, map[string]string{
		"package.json":   "{\"dependencies\":{}}\n",
		".gitattributes": "* filter=evil diff=evil\n",
	})
	marker = filepath.Join(t.TempDir(), "ran")
	cfg := filepath.Join(dir, ".git", "config")
	f, err := os.OpenFile(cfg, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G304: a path inside the test's repository
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	prog := "touch " + marker + "; cat"
	if _, err := f.WriteString("[filter \"evil\"]\n\tclean = " + prog + "\n\tsmudge = " + prog + "\n[diff \"evil\"]\n\ttextconv = " + prog + "\n\tcommand = " + prog + "\n[core]\n\tfsmonitor = " + prog + "\n"); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "package.json"), "{\"dependencies\":{\"left-pad\":\"^1.0.0\"}}\n")
	write(t, filepath.Join(dir, "web", "package.json"), "{\"dependencies\":{\"@scope/pkg\":\"^2.0.0\"}}\n")
	return dir, base, marker
}

// T6: the agent owns the checkout, so nothing in its git configuration may start a program
// while the dependency rule reads it.
func TestNoProgramFromTheCheckoutConfigurationRuns(t *testing.T) {
	dir, base, marker := evilCheckout(t)
	got, err := DependencyFindings(t.Context(), dir, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a program from the checkout's git configuration ran")
	}
	if len(got) != 2 {
		t.Fatalf("the rule must still find both new dependencies: %+v", got)
	}
}

func TestGitRunsAsTheAgentUser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to drop to another user")
	}
	dir, base, _ := evilCheckout(t)
	if err := exec.CommandContext(t.Context(), "chmod", "-R", "a+rX", filepath.Dir(dir)).Run(); err != nil { //nolint:gosec // G204: fixed arguments in a test
		t.Fatal(err)
	}
	got, err := DependencyFindings(t.Context(), dir, base, &Agent{UID: 65534, GID: 65534, Home: "/nonexistent"})
	if err != nil || len(got) != 2 {
		t.Fatalf("as the agent user: %+v, err %v", got, err)
	}
}
