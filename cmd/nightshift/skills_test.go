package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsSyncEmptyList(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "vendor.yaml")
	if err := os.WriteFile(f, []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := skillsCmd([]string{"sync", "--file", f, "--dir", filepath.Join(dir, "vendor")}, &out, &errb); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if code := skillsCmd([]string{"bogus"}, &out, &errb); code != exitUsage {
		t.Fatalf("usage exit %d", code)
	}
}
