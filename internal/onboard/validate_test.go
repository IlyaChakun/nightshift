package onboard_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/onboard"
)

const goodProject = `name: demo
stack: [go]
instructions: [AGENTS.md]
skills: [.nightshift/skills/]
verify: make verify
capabilities: { merge_train: true }
blast_radius: .nightshift/blast-radius.yaml
`

func repo(project string, extra map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		".nightshift/project.yaml":      {Data: []byte(project)},
		"AGENTS.md":                     {Data: []byte("# demo\n")},
		".nightshift/skills/a/SKILL.md": {Data: []byte("---\nname: a\n---\n")},
	}
	for k, v := range extra {
		fsys[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return fsys
}

func TestValidateProject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fsys    fstest.MapFS
		tenant  string
		packs   []string
		want    []string // substrings, one per problem
		wantErr bool
	}{
		{name: "missing file", fsys: fstest.MapFS{}, want: []string{"the file is missing"}, wantErr: true},
		{name: "schema error", fsys: repo("name: demo\n", nil), want: []string{"project.yaml"}, wantErr: true},
		{name: "missing blast radius file", fsys: repo(goodProject, nil), packs: []string{"go"}, want: []string{"blast_radius"}, wantErr: true},
		{name: "unknown stack is a warning", fsys: repo(goodProject, nil), packs: nil, want: []string{"no stack pack \"go\"", "blast_radius"}, wantErr: true},
		{name: "missing instruction", fsys: repo(strings.Replace(goodProject, "AGENTS.md", "NOPE.md", 1), nil), packs: []string{"go"}, want: []string{"NOPE.md", "blast_radius"}, wantErr: true},
		{name: "wrong tenant name", fsys: repo(goodProject, nil), tenant: "other", packs: []string{"go"}, want: []string{"the tenant is \"other\"", "blast_radius"}, wantErr: true},
		{name: "path outside the repository", fsys: repo(strings.Replace(goodProject, ".nightshift/blast-radius.yaml", "../x.yaml", 1), nil), packs: []string{"go"}, want: []string{"leaves the repository"}, wantErr: true},
	}
	v := config.NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := onboard.ValidateProject(tt.fsys, v, tt.packs, tt.tenant)
			if len(got) != len(tt.want) {
				t.Fatalf("problems = %v, want %d matching %v", got, len(tt.want), tt.want)
			}
			text := ""
			for _, p := range got {
				text += p.String() + "\n"
			}
			for _, w := range tt.want {
				if !strings.Contains(text, w) {
					t.Errorf("no problem mentions %q in:\n%s", w, text)
				}
			}
			if onboard.HasErrors(got) != tt.wantErr {
				t.Errorf("HasErrors = %v, want %v", !tt.wantErr, tt.wantErr)
			}
		})
	}
}

func TestValidateProjectExampleSolo(t *testing.T) {
	t.Parallel()
	// the synthetic tenant is the reference for a valid project; its pointers need files
	fsys := fstest.MapFS{
		"AGENTS.md":                     {Data: []byte("# example\n")},
		".nightshift/skills/a/SKILL.md": {Data: []byte("---\nname: a\n---\n")},
	}
	for _, f := range []string{"project.yaml", "blast-radius.yaml"} {
		b, err := os.ReadFile("../../testdata/tenants/example-solo/.nightshift/" + f)
		if err != nil {
			t.Fatal(err)
		}
		fsys[".nightshift/"+f] = &fstest.MapFile{Data: b}
	}
	for _, p := range onboard.ValidateProject(fsys, config.NewValidator(), []string{"go", "python", "ts"}, "example-solo") {
		t.Errorf("unexpected problem: %s", p)
	}
}
