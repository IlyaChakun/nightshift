package main

import (
	"bytes"
	"strings"
	"testing"
)

// T1, review finding on ADR 0028: a bad prefix stops the deployer at start, not on every preview.
func TestDeployerRefusesABadProjectPrefixAtStart(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"NS", "ns_x", "a-prefix-longer-than-16"} {
		var stderr bytes.Buffer
		if got := deployerCmd([]string{"--work", t.TempDir(), "--project-prefix", p}, &stderr); got != exitUsage || !strings.Contains(stderr.String(), "--project-prefix") {
			t.Errorf("%q: exit %d, %s", p, got, stderr.String())
		}
	}
}
