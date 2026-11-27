package codex_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/codex"
	"github.com/ilyachakun/nightshift/internal/execution/harness/harnesstest"
)

// TestContract runs the harness contract suite on the adapter, with a stub CLI per reply.
func TestContract(t *testing.T) {
	harnesstest.Run(t, func(t *testing.T, reply harnesstest.Reply) harness.Harness {
		t.Helper()
		var bin string
		switch reply {
		case harnesstest.Success:
			bin, _ = stub(t, done, `{"summary":"done"}`, 0)
		case harnesstest.Limit:
			bin, _ = stub(t, `{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}`, "", 1)
		case harnesstest.Failure:
			bin, _ = stub(t, `{"type":"error","message":"boom"}`, "", 1)
		case harnesstest.Garbage:
			bin, _ = stub(t, `not json`, "", 1)
		case harnesstest.GarbageExitZero:
			bin, _ = stub(t, `not json`, "", 0)
		case harnesstest.Hang:
			bin = harnesstest.HangStub(t, "codex")
		}
		return &codex.Harness{Bin: bin}
	})
}
