package main

import (
	"bytes"
	"testing"
)

func TestMemoryUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {"show"}, {"forget", "t"}, {"remember", "t"}} {
		var out, errb bytes.Buffer
		if got := memoryCmd(args, &out, &errb); got != exitUsage {
			t.Errorf("memory %v = %d, want usage", args, got)
		}
	}
}
