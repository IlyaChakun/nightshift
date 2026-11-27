package standards_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ilyachakun/nightshift/internal/assurance/standards"
)

var errRead = errors.New("read failed")

// failingFS fails to open one file, as a damaged checkout would.
type failingFS struct {
	fstest.MapFS
	bad string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.bad {
		return nil, errRead
	}
	return f.MapFS.Open(name)
}

func TestErrorPaths(t *testing.T) {
	t.Parallel()
	good := fstest.MapFS{
		"industry/go/a.yaml":       {Data: []byte("id: a\ntitle: A\nstatus: active\n")},
		"industry/go/files/cfg.md": {Data: []byte("x")},
	}
	broken := fstest.MapFS{"industry/go/a.yaml": {Data: []byte(": :")}}
	unreadable := failingFS{MapFS: good, bad: "industry/go/a.yaml"}
	unreadableCfg := failingFS{MapFS: good, bad: "industry/go/files/cfg.md"}
	cfg := func(name string) fstest.MapFS {
		return fstest.MapFS{"industry/go/files/" + name: {Data: []byte("x")}}
	}

	tests := []struct {
		name string
		run  func(t *testing.T) error
	}{
		{"checklist read", func(*testing.T) error {
			_, err := standards.Checklist(unreadable, []string{"go"}, "/standards")
			return err
		}},
		{"list bad yaml", func(*testing.T) error { _, err := standards.List(broken, nil); return err }},
		{"list read", func(*testing.T) error { _, err := standards.List(unreadable, nil); return err }},
		{"sync read", func(t *testing.T) error {
			_, _, err := standards.Sync(unreadableCfg, []string{"go"}, t.TempDir())
			return err
		}},
		{"sync mkdir", func(t *testing.T) error {
			dest := t.TempDir()
			if err := os.WriteFile(filepath.Join(dest, "sub"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := standards.Sync(cfg("sub/x.cfg"), []string{"go"}, dest)
			return err
		}},
		{"sync write", func(t *testing.T) error {
			dest := t.TempDir()
			// A dangling symlink is not found by Stat, but writing through it fails.
			if err := os.Symlink(filepath.Join(dest, "missing", "t"), filepath.Join(dest, "y.cfg")); err != nil {
				t.Fatal(err)
			}
			_, _, err := standards.Sync(cfg("y.cfg"), []string{"go"}, dest)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.run(t); err == nil {
				t.Fatal("error was swallowed")
			}
		})
	}
}

// ReadFile is overridden too: MapFS implements it without calling Open.
func (f failingFS) ReadFile(name string) ([]byte, error) {
	if name == f.bad {
		return nil, errRead
	}
	return f.MapFS.ReadFile(name)
}
