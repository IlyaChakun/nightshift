package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // test helper with fixed arguments
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // git must be found
		"HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestOpenOnboardingPR(t *testing.T) {
	up := t.TempDir()
	gitIn(t, up, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(up, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, up, "add", ".")
	gitIn(t, up, "commit", "-q", "-m", "init")

	twin := fake.New(t, "tok")
	ref, err := openOnboardingPR(t.Context(), onboardPR{
		Repo: "example/app", Base: "main", Remote: up, Token: "tok",
		Files: map[string][]byte{".nightshift/project.yaml": []byte("name: app\ncommands:\n  verify: make test\n")},
		Body:  "gaps", Forge: &github.Client{BaseURL: twin.URL, Token: "tok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pr, ok := twin.PR("example/app", ref.Number)
	if !ok || !pr.Draft || pr.Head != "ns/onboard" || pr.Base != "main" {
		t.Fatalf("pull request: %+v", pr)
	}
	if got := gitIn(t, up, "show", "ns/onboard:.nightshift/project.yaml"); !strings.Contains(got, "verify: make test") {
		t.Fatalf("pushed file: %q", got)
	}
	if got := gitIn(t, up, "rev-parse", "main"); got == gitIn(t, up, "rev-parse", "ns/onboard") {
		t.Fatal("the branch has no commit of its own")
	}
}

func TestNewFilesPatchCountsLines(t *testing.T) {
	p := string(newFilesPatch(map[string][]byte{"b.md": []byte("x\ny"), "a.md": []byte("z\n")}))
	if !strings.Contains(p, "+++ b/a.md\n@@ -0,0 +1,1 @@\n+z\n") || !strings.Contains(p, "@@ -0,0 +1,2 @@\n+x\n+y\n") || strings.Index(p, "a.md") > strings.Index(p, "b.md") {
		t.Fatalf("patch:\n%s", p)
	}
}
