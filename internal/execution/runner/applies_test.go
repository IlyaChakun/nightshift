package runner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDependencySkillsFollowThePackageJSON(t *testing.T) {
	skills, ws := t.TempDir(), t.TempDir()
	write(t, filepath.Join(skills, "own", "web-dev", "SKILL.md"), "---\nname: web-dev\napplies_to: { deps: [react, vue], unless_deps: [react-native] }\nroles: [builder]\n---\n# web\n")
	write(t, filepath.Join(skills, "own", "mobile-dev", "SKILL.md"), "---\nname: mobile-dev\napplies_to: { deps: [react-native] }\n---\n# mobile\n")
	write(t, filepath.Join(skills, "own", "audit", "SKILL.md"), "---\nname: audit\n---\n# no selector\n")
	write(t, filepath.Join(ws, "package.json"), `{"dependencies":{"react":"18"},"devDependencies":{"react-native":"0.76"}}`)
	r := &Runner{SkillsDir: skills}

	if got := r.dependencySkills("builder", ws); !slices.Equal(got, []string{"own/mobile-dev"}) {
		t.Fatalf("builder: %v", got)
	}
	web := t.TempDir()
	write(t, filepath.Join(web, "package.json"), `{"dependencies":{"react":"18"}}`)
	if got := r.dependencySkills("builder", web); !slices.Equal(got, []string{"own/web-dev"}) {
		t.Fatalf("a web project: %v", got)
	}
	if got := r.dependencySkills("verifier", web); len(got) != 0 {
		t.Fatalf("a skill limited to builder must not reach the verifier: %v", got)
	}
	if got := r.dependencySkills("verifier", ws); !slices.Equal(got, []string{"own/mobile-dev"}) {
		t.Fatalf("a skill limited to builder must not reach the verifier: %v", got)
	}
	if got := r.dependencySkills("builder", t.TempDir()); len(got) != 0 {
		t.Fatalf("a project without package.json selects nothing: %v", got)
	}
}

func TestShippedFrameworkSkillsNameTheirDependencies(t *testing.T) {
	for name, dep := range map[string]string{"web-dev": "react", "mobile-dev": "react-native"} {
		m, ok := readSkillMeta(filepath.Join("..", "..", "..", "skills", "own", name, "SKILL.md"))
		if !ok || !slices.Contains(m.AppliesTo.Deps, dep) || !slices.Contains(m.Roles, "builder") {
			t.Errorf("%s: meta %+v ok=%v", name, m, ok)
		}
	}
}
