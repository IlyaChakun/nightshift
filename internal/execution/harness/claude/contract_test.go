package claude_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/harness"
	"github.com/ilyachakun/nightshift/internal/execution/harness/claude"
	"github.com/ilyachakun/nightshift/internal/execution/harness/harnesstest"
)

// TestContract runs the harness contract suite on the adapter, with a stub CLI per reply.
func TestContract(t *testing.T) {
	harnesstest.Run(t, func(t *testing.T, reply harnesstest.Reply) harness.Harness {
		t.Helper()
		var bin string
		switch reply {
		case harnesstest.Success:
			bin, _ = stub(t, `{"type":"result","subtype":"success","is_error":false,"result":"ok","structured_output":{"summary":"done"},"session_id":"s-1","usage":{"input_tokens":10,"output_tokens":5}}`, 0)
		case harnesstest.Limit:
			bin, _ = stub(t, `{"type":"result","subtype":"success","is_error":true,"result":"Claude usage limit reached. Resets at 5pm."}`, 1)
		case harnesstest.Failure:
			bin, _ = stub(t, `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`, 1)
		case harnesstest.Garbage:
			bin, _ = stub(t, `not json`, 1)
		case harnesstest.GarbageExitZero:
			bin, _ = stub(t, `not json`, 0)
		case harnesstest.Hang:
			bin = harnesstest.HangStub(t, "claude")
		}
		return &claude.Harness{Bin: bin, Auth: claude.Subscription}
	})
}
