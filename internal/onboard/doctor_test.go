package onboard_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/onboard"
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
