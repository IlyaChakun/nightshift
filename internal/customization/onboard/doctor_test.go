package onboard_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/customization/onboard"
	"github.com/ilyachakun/nightshift/internal/customization/stack"
)

const validProject = `name: demo
stack: [go]
verify: make verify
capabilities: { merge_train: true }
blast_radius: .nightshift/blast-radius.yaml
`

func byName(rs []onboard.Result, name string) (onboard.Result, bool) {
	for _, r := range rs {
		if r.Name == name {
			return r, true
		}
	}
	return onboard.Result{}, false
}

func doctor(repo fstest.MapFS, missingTool string, missingSecret string, chat string) onboard.Doctor {
	var t config.Tenant
	t.Chat.Adapter = chat
	return onboard.Doctor{
		Tenant: t, Repo: repo, Validator: config.NewValidator(),
		Look: func(n string) error {
			if n == missingTool {
				return errors.New("not found")
			}
			return nil
		},
		Secret: func(n string) error {
			if n == missingSecret {
				return errors.New("missing")
			}
			return nil
		},
	}
}

func TestDoctorPassesAConnectedProject(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{
		".nightshift/project.yaml": file(validProject), "AGENTS.md": file("x"), ".golangci.yml": file("x"),
		"mise.toml": file("x"), ".github/workflows/ci.yml": file("x"), "SECURITY.md": file("x"),
	}
	rs := doctor(repo, "", "", "chat-app").Run()
	if onboard.Failed(rs) {
		t.Fatalf("a connected project failed:\n%s", onboard.Format(rs))
	}
	for _, r := range rs {
		if r.Status == onboard.Warn {
			t.Errorf("unexpected warning: %+v", r)
		}
	}
}

func TestDoctorFailsOnWhatBlocksAndWarnsOnWhatIsMissing(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{".nightshift/project.yaml": file(validProject)}
	rs := doctor(repo, "go", "chat_app_token", "chat-app").Run()
	for _, name := range []string{"tool go", "secret chat_app_token"} {
		if r, ok := byName(rs, name); !ok || r.Status != onboard.Fail {
			t.Errorf("%s: %+v %v", name, r, ok)
		}
	}
	if r, _ := byName(rs, "ready: project instructions"); r.Status != onboard.Warn || r.Fix == "" {
		t.Errorf("instructions: %+v", r)
	}
	if !strings.Contains(onboard.Format(rs), "fix: ") {
		t.Error("the report names no fixes")
	}
	// github-only chat needs no chat tokens.
	rs = doctor(repo, "", "chat_bot_token", "github-only").Run()
	if _, ok := byName(rs, "secret chat_bot_token"); ok {
		t.Error("a github-only tenant was asked for a chat token")
	}
	// The inbox lives in the book.
	rs = doctor(repo, "", "chat_bot_token", "inbox").Run()
	if _, ok := byName(rs, "secret chat_bot_token"); ok {
		t.Error("an inbox tenant was asked for a chat token")
	}
}

func TestDoctorFailsWithoutAValidProjectFile(t *testing.T) {
	t.Parallel()
	for name, repo := range map[string]fstest.MapFS{
		"missing": {},
		"invalid": {".nightshift/project.yaml": file("name: Bad Name\n")},
	} {
		rs := doctor(repo, "", "", "").Run()
		if r, _ := byName(rs, "project.yaml"); r.Status != onboard.Fail {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestDoctorAcceptsAPinnedToolchainAndWarnsOnAStackWithoutAPack(t *testing.T) {
	t.Parallel()
	project := "name: demo\nstack: [rust]\nverify: make verify\ncapabilities: { merge_train: true }\nblast_radius: x\n"
	repo := fstest.MapFS{".nightshift/project.yaml": file(project), "mise.toml": file("[tools]\nrust = \"1.90\"\n")}
	d := doctor(repo, "cargo", "", "chat-app") // the worker has no cargo, and needs none: mise installs it
	d.Packs = stack.Packs{"go": {Name: "go"}}
	rs := d.Run()
	if r, _ := byName(rs, "toolchain"); r.Status != onboard.OK || !strings.Contains(r.Detail, "mise.toml") {
		t.Fatalf("toolchain: %+v\n%s", r, onboard.Format(rs))
	}
	if r, ok := byName(rs, "stack rust"); !ok || r.Status != onboard.Warn || !strings.Contains(r.Fix, ".nightshift/stacks/rust") {
		t.Fatalf("stack rust: %+v", r)
	}
	if _, ok := byName(rs, "tool cargo"); ok {
		t.Fatal("a pinned project was asked for a toolchain binary")
	}

	// Without mise on the worker, a pinned project fails; without a pin, a missing toolchain names mise.toml as the fix.
	if rs := doctor(repo, "mise", "", "chat-app").Run(); !onboard.Failed(rs) {
		t.Fatalf("a pinned toolchain without mise passed:\n%s", onboard.Format(rs))
	}
	bare := fstest.MapFS{".nightshift/project.yaml": file(validProject)}
	rs = doctor(bare, "go", "", "chat-app").Run()
	if r, _ := byName(rs, "tool go"); r.Status != onboard.Fail || !strings.Contains(r.Fix, "mise.toml") {
		t.Fatalf("tool go: %+v", r)
	}
}

func TestDoctorCountsAProjectPackAndRefusesABrokenOne(t *testing.T) {
	t.Parallel()
	packs := stack.Packs{"go": {Name: "go"}}
	repo := fstest.MapFS{
		".nightshift/project.yaml":          file("name: demo\nstack: [lang]\nverify: make verify\ncapabilities: { merge_train: true }\nblast_radius: .nightshift/blast-radius.yaml\n"),
		".nightshift/stacks/lang/pack.yaml": file("name: lang\ndetect: [lang.cfg]\ntoolchain: [make]\ntools: []\n"),
		"mise.toml":                         file("x"),
	}
	d := doctor(repo, "", "", "inbox")
	d.Packs = packs
	rs := d.Run()
	if _, warned := byName(rs, "stack lang"); warned || onboard.Failed(rs) {
		t.Fatalf("a stack with a project pack needs no warning:\n%s", onboard.Format(rs))
	}
	repo[".nightshift/stacks/lang/pack.yaml"] = file("name: other\n")
	d = doctor(repo, "", "", "inbox")
	d.Packs = packs
	if rs := d.Run(); !onboard.Failed(rs) {
		t.Fatalf("a broken project pack must fail the doctor:\n%s", onboard.Format(rs))
	}
}

func TestDoctorWarnsWhenAProjectPackReplacesAFactoryPack(t *testing.T) {
	t.Parallel()
	repo := fstest.MapFS{
		".nightshift/project.yaml":        file("name: demo\nstack: [go]\nverify: make verify\ncapabilities: { merge_train: true }\nblast_radius: .nightshift/blast-radius.yaml\n"),
		".nightshift/stacks/go/pack.yaml": file("name: go\ndetect: [go.mod]\ntoolchain: [make]\ntools: []\n"),
		"mise.toml":                       file("x"),
	}
	d := doctor(repo, "", "", "inbox")
	d.Packs = stack.Packs{"go": {Name: "go"}}
	if r, ok := byName(d.Run(), "stack pack go"); !ok || r.Status != onboard.Warn {
		t.Fatalf("a replaced factory pack must be a warning: %+v", r)
	}
}
