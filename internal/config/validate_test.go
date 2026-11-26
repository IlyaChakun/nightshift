package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kindOf maps a testdata file to the schema it must satisfy.
func kindOf(path string) (Kind, bool) {
	base := filepath.Base(path)
	switch {
	case base == "tenant.yaml":
		return KindTenant, true
	case base == "project.yaml":
		return KindProject, true
	case base == "blast-radius.yaml":
		return KindBlastRadius, true
	case base == "instance.yaml":
		return KindInstance, true
	case base == "providers.yaml":
		return KindProviders, true
	case filepath.Base(filepath.Dir(path)) == "roles":
		return KindRole, true
	}
	return "", false
}

// TestTestdataConfigsAreValid is the phase 0 "Done" check: every config
// example, including the synthetic tenants, validates against its schema.
func TestTestdataConfigsAreValid(t *testing.T) {
	t.Parallel()
	v := NewValidator()
	var checked int
	root := os.DirFS("../../testdata")
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		kind, ok := kindOf(path)
		if !ok {
			return nil
		}
		checked++
		doc, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		if err := v.ValidateYAML(kind, doc); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Fatalf("checked %d config files, want at least 10", checked)
	}
}

func TestInvalidConfigsAreRejected(t *testing.T) {
	t.Parallel()
	const team = `
name: t
governance: team
data_classification: internal
repos: [github.com/o/r]
people: { owner: { github: o }, approvers: [owner] }
chat: { adapter: example-chat }
tracker: { adapter: example-tracker }
docs: { adapter: git }
providers: { builder: [p] }
schedule: { timezone: Etc/UTC }
`
	tests := []struct {
		name string
		kind Kind
		doc  string
	}{
		{"team cannot switch review off", KindTenant, team + "review: { mode: off }\n"},
		{"team has no SLA auto-merge", KindTenant, team + "review: { mode: all, sla_automerge_hours: 12 }\n"},
		{"critical is always reviewed", KindTenant, team + "review: { mode: all, actions: { critical: auto } }\n"},
		{"the remote MCP door must listen on loopback", KindTenant, team + "review: { mode: all }\nmcp: { remote: { listen: \"0.0.0.0:8765\" } }\n"},
		{"unknown tenant field", KindTenant, team + "review: { mode: all }\nmerge_by_bot: true\n"},
		{"artifacts are in English", KindTenant, team + "review: { mode: all }\nlanguage: { artifacts: ru }\n"},
		{"subscription runs one session", KindProviders, "sub: { harness: claude, auth: subscription, concurrency: 2 }\n"},
		{"subscription is never batched", KindProviders, "sub: { harness: claude, auth: subscription, concurrency: 1, batch: allowed }\n"},
		{"unknown auth", KindProviders, "p: { harness: claude, auth: password }\n"},
		{"preview script needs down", KindProject, "name: p\nstack: [go]\nverify: make verify\nblast_radius: b.yaml\ncapabilities: { merge_train: true, preview: { kind: script, up: ./up.sh } }\n"},
		{"compose-local preview needs file and entry", KindProject, projectWith("preview: { kind: compose-local, file: deploy/compose.yaml }")},
		{"compose-local recipe is not absolute", KindProject, projectWith("preview: { kind: compose-local, file: /etc/compose.yaml, entry: web }")},
		{"compose-local recipe cannot leave the repository", KindProject, projectWith("preview: { kind: compose-local, file: ../x.yaml, entry: web }")},
		{"compose-local recipe cannot climb in the middle", KindProject, projectWith("release: { kind: compose-local, file: a/../../b.yaml }")},
		{"compose-local recipe is a file, not a directory", KindProject, projectWith("release: { kind: compose-local, file: deploy/ }")},
		{"compose-local limits memory has a unit", KindProject, projectWith("preview: { kind: compose-local, file: c.yaml, entry: web, limits: { memory: lots } }")},
		{"unknown deploy kind", KindProject, projectWith("release: { kind: kubernetes }")},
		{"unknown blast level", KindBlastRadius, "levels: { extreme: { paths: [x] } }\n"},
		{"role access is closed", KindRole, "name: r\non: [task.x]\naccess: admin\noutput_schema: schemas/verdict.json\nlimits: { minutes: 1, tokens: 1 }\n"},
		{"two yaml documents", KindRole, "name: a\n---\nname: b\n"},
	}
	v := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := v.ValidateYAML(tt.kind, []byte(tt.doc)); err == nil {
				t.Errorf("ValidateYAML accepted an invalid %s", tt.kind)
			} else if testing.Verbose() {
				t.Log(err)
			}
		})
	}
}

func TestTheRemoteMCPDoorIsOneLineInTenantYAML(t *testing.T) {
	t.Parallel()
	const doc = `
name: t
governance: solo
data_classification: internal
repos: [github.com/o/r]
people: { owner: { github: o }, approvers: [owner] }
chat: { adapter: inbox }
tracker: { adapter: example-tracker }
docs: { adapter: git }
providers: { builder: [p] }
schedule: { timezone: Etc/UTC }
review: { mode: all }
mcp: { remote: { listen: "127.0.0.1:8765" } }
`
	if err := NewValidator().ValidateYAML(KindTenant, []byte(doc)); err != nil {
		t.Fatal(err)
	}
}

// A tenant raises or removes the supervisor's limits for its own runs (ADR 0019).
func TestTenantRunLimits(t *testing.T) {
	t.Parallel()
	const head = `
name: t
governance: solo
data_classification: internal
repos: [github.com/o/r]
people: { owner: { github: o }, approvers: [owner] }
chat: { adapter: inbox }
tracker: { adapter: example-tracker }
docs: { adapter: git }
providers: { builder: [p] }
schedule: { timezone: Etc/UTC }
review: { mode: all }
`
	v := NewValidator()
	if err := v.ValidateYAML(KindTenant, []byte(head+"budget: { run: { minutes: 90, stall_minutes: 0, repeat_limit: 10, cost_usd: 5.5 } }\n")); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]string{"unknown key": "{ idle: 1 }", "negative": "{ stall_minutes: -1 }"} {
		if err := v.ValidateYAML(KindTenant, []byte(head+"budget: { run: "+run+" }\n")); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestUnknownKind(t *testing.T) {
	t.Parallel()
	if err := NewValidator().ValidateYAML("nope", []byte("a: 1\n")); err == nil {
		t.Fatal("want an error for an unknown schema kind")
	}
}

// TestShippedRolesAreValid checks the role configs in roles/.
func TestShippedRolesAreValid(t *testing.T) {
	t.Parallel()
	root := os.DirFS("../../roles")
	files, err := fs.Glob(root, "*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no role configs in roles/")
	}
	v := NewValidator()
	for _, name := range files {
		doc, err := fs.ReadFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidateYAML(KindRole, doc); err != nil {
			t.Errorf("roles/%s: %v", name, err)
		}
	}
}

func projectWith(caps string) string {
	return "name: p\nstack: [go]\nverify: make verify\nblast_radius: b.yaml\ncapabilities: { merge_train: true, " + caps + " }\n"
}

func TestComposeLocalCapabilitiesAreValid(t *testing.T) {
	t.Parallel()
	doc := "name: p\nstack: [go]\nverify: make verify\nblast_radius: b.yaml\ncapabilities:\n" +
		"  merge_train: true\n" +
		"  preview: { kind: compose-local, file: deploy/compose.yaml, entry: web, limits: { cpus: 2, memory: 2g } }\n" +
		"  release: { kind: compose-local, file: deploy/compose.yaml }\n" +
		"  rollback: { kind: compose-local }\n" +
		"  smoke: ./scripts/smoke-main-path.sh\n"
	if err := NewValidator().ValidateYAML(KindProject, []byte(doc)); err != nil {
		t.Fatal(err)
	}
}
