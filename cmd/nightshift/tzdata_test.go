package main

import (
	"os/exec"
	"strings"
	"testing"
)

// A test machine has its own time zone database, so loading a zone proves nothing here: the
// binary must carry one, because the images do not.
func TestTheBinaryCarriesTheTimeZones(t *testing.T) {
	t.Parallel()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "\ntime/tzdata\n") {
		t.Fatal("time/tzdata is not linked in")
	}
}
