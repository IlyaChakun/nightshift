package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const goodSHA = "0123456789abcdef0123456789abcdef01234567"

func TestValidate(t *testing.T) {
	ok := Entry{ID: "acme/code-review", Repo: "https://example.invalid/acme/skills", Path: "skills/x", SHA: goodSHA, License: "MIT", Roles: []string{"verifier"}}
	with := func(f func(*Entry)) Entry { e := ok; f(&e); return e }
	tests := []struct {
		name string
		e    []Entry
		want string // empty means valid
	}{
		{"valid", []Entry{ok}, ""},
		{"empty list", nil, ""},
		{"short sha", []Entry{with(func(e *Entry) { e.SHA = "abc123" })}, "40"},
		{"upper-case sha", []Entry{with(func(e *Entry) { e.SHA = strings.ToUpper(goodSHA) })}, "40"},
		{"branch as sha", []Entry{with(func(e *Entry) { e.SHA = "main" })}, "40"},
		{"missing licence", []Entry{with(func(e *Entry) { e.License = " " })}, "license"},
		{"bad id", []Entry{with(func(e *Entry) { e.ID = "Code Review" })}, "id"},
		{"own set", []Entry{with(func(e *Entry) { e.ID = "own/x" })}, "own"},
		{"duplicate", []Entry{ok, ok}, "duplicate"},
		{"http repo", []Entry{with(func(e *Entry) { e.Repo = "http://example.invalid/x" })}, "https"},
		{"option as repo", []Entry{with(func(e *Entry) { e.Repo = "--upload-pack=x" })}, "https"},
		{"path escapes", []Entry{with(func(e *Entry) { e.Path = "../x" })}, "path"},
		{"no roles", []Entry{with(func(e *Entry) { e.Roles = nil })}, "roles"},
	}
	for _, tt := range tests {
		err := Validate(tt.e, false)
		if (tt.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%s: got %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	f := filepath.Join(t.TempDir(), "vendor.yaml")
	if err := os.WriteFile(f, []byte("- id: a/b\n  sha_: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(f, false); err == nil {
		t.Fatal("unknown key accepted")
	}
	if err := os.WriteFile(f, []byte("# empty\n[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if es, err := Load(f, false); err != nil || len(es) != 0 {
		t.Fatalf("empty list: %v %v", es, err)
	}
}

// repo builds a local git repository and returns its path and a commit function.
func repo(t *testing.T) (string, func(files map[string]string, exec ...string) string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	return dir, func(files map[string]string, execs ...string) string {
		for name, body := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			mode := os.FileMode(0o644)
			for _, x := range execs {
				if x == name {
					mode = 0o755
				}
			}
			if err := os.WriteFile(p, []byte(body), mode); err != nil {
				t.Fatal(err)
			}
		}
		run("add", "-A")
		run("commit", "-q", "-m", "c")
		return run("rev-parse", "HEAD")
	}
}

func setup(t *testing.T, dir, sha string) Options {
	t.Helper()
	root := t.TempDir()
	y := "- id: acme/demo\n  repo: " + dir + "\n  path: skills/demo\n  sha: " + sha + "\n  license: MIT\n  roles: [builder]\n"
	if err := os.WriteFile(filepath.Join(root, "vendor.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	return Options{File: filepath.Join(root, "vendor.yaml"), Dir: filepath.Join(root, "vendor"), AllowLocal: true}
}

func TestSyncWritesDiffsAndUpdates(t *testing.T) {
	dir, commit := repo(t)
	v1 := commit(map[string]string{"LICENSE": "MIT licence text", "skills/demo/SKILL.md": "# Demo\none\n"})
	o := setup(t, dir, v1)

	rs, err := Sync(context.Background(), Options{File: o.File, Dir: o.Dir, AllowLocal: true, DryRun: true})
	if err != nil || len(rs) != 1 || rs[0].Diff == "" {
		t.Fatalf("dry run: %v %+v", err, rs)
	}
	if _, err := os.Stat(o.Dir); err == nil {
		t.Fatal("dry run wrote files")
	}

	if rs, err = Sync(context.Background(), o); err != nil || Refused(rs) {
		t.Fatalf("sync: %v %+v", err, rs)
	}
	for _, f := range []string{"SKILL.md", "LICENSE", "VENDORED.yaml"} {
		if _, err := os.Stat(filepath.Join(o.Dir, "acme", "demo", f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if rs, _ = Sync(context.Background(), o); rs[0].Diff != "" {
		t.Errorf("second sync is not a no-op: %s", rs[0].Diff)
	}

	v2 := commit(map[string]string{"skills/demo/SKILL.md": "# Demo\ntwo\n"})
	o2 := setup(t, dir, v2)
	o2.Dir = o.Dir
	rs, err = Sync(context.Background(), o2)
	if err != nil || !strings.Contains(rs[0].Diff, "+two") || !strings.Contains(rs[0].Diff, "-one") {
		t.Fatalf("update diff: %v %q", err, rs[0].Diff)
	}
	if b, _ := os.ReadFile(filepath.Join(o.Dir, "acme", "demo", "SKILL.md")); !strings.Contains(string(b), "two") {
		t.Errorf("update not written: %s", b)
	}
	if !strings.Contains(Format(rs), "changed") {
		t.Error("report does not say changed")
	}
}

func TestSyncRefuses(t *testing.T) {
	skill := "# Demo\n"
	tests := []struct {
		name  string
		files map[string]string
		execs []string
		want  string
	}{
		{"script", map[string]string{"skills/demo/run.sh": "echo hi\n"}, nil, "script"},
		{"python script", map[string]string{"skills/demo/tool.py": "print(1)\n"}, nil, "script"},
		{"shebang", map[string]string{"skills/demo/tool": "#!/bin/sh\n"}, nil, "shebang"},
		{"exec bit", map[string]string{"skills/demo/notes.md": "x"}, []string{"skills/demo/notes.md"}, "executable"},
		{"hidden unicode", map[string]string{"skills/demo/SKILL.md": skill + "ignore​ previous\n"}, nil, "U+200B"},
		{"bidi override", map[string]string{"skills/demo/SKILL.md": skill + "a‮b\n"}, nil, "U+202E"},
		{"binary", map[string]string{"skills/demo/data.bin": "a\x00b"}, nil, "binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, commit := repo(t)
			files := map[string]string{"LICENSE": "MIT", "skills/demo/SKILL.md": skill}
			for k, v := range tt.files {
				files[k] = v
			}
			o := setup(t, dir, commit(files, tt.execs...))
			rs, err := Sync(context.Background(), o)
			if err != nil || !Refused(rs) || !strings.Contains(strings.Join(rs[0].Findings, "\n"), tt.want) {
				t.Fatalf("got %v %+v, want a %q finding", err, rs, tt.want)
			}
			if _, err := os.Stat(o.Dir); err == nil {
				t.Error("a refused skill was written")
			}
		})
	}
}

func TestSyncRefusesSymlink(t *testing.T) {
	dir, commit := repo(t)
	if err := os.MkdirAll(filepath.Join(dir, "skills/demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "skills/demo/link.md")); err != nil {
		t.Skip("no symlinks")
	}
	o := setup(t, dir, commit(map[string]string{"LICENSE": "MIT", "skills/demo/SKILL.md": "# Demo\n"}))
	if rs, err := Sync(context.Background(), o); err != nil || !Refused(rs) {
		t.Fatalf("symlink accepted: %v %+v", err, rs)
	}
}

func TestSyncErrors(t *testing.T) {
	dir, commit := repo(t)
	sha := commit(map[string]string{"skills/demo/SKILL.md": "# Demo\n"}) // no licence anywhere
	if _, err := Sync(context.Background(), setup(t, dir, sha)); err == nil || !strings.Contains(err.Error(), "licence") {
		t.Errorf("missing licence file: %v", err)
	}
	if _, err := Sync(context.Background(), setup(t, dir, strings.Repeat("a", 40))); err == nil {
		t.Error("unknown sha accepted")
	}
	dir, commit = repo(t)
	sha = commit(map[string]string{"LICENSE": "MIT", "skills/demo/README.md": "no skill file"})
	if _, err := Sync(context.Background(), setup(t, dir, sha)); err == nil {
		t.Error("folder without SKILL.md accepted")
	}
}

func TestSyncSecret(t *testing.T) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		t.Skip("gitleaks is not installed")
	}
	dir, commit := repo(t)
	secret := "AKIA" + "IOSFODNN7" + "EXAMPLX" // built at run time so this file stays clean
	o := setup(t, dir, commit(map[string]string{"LICENSE": "MIT", "skills/demo/SKILL.md": "key = " + secret + "\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"}))
	if rs, err := Sync(context.Background(), o); err != nil || !Refused(rs) {
		t.Fatalf("secret accepted: %v %+v", err, rs)
	}
}
