package gitops_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/gitops"
)

// A step cut by a timeout must not leave lock files in the mirror (plan R1): the canceled git
// is asked to stop with SIGTERM, which lets it clean up, and is killed only when it ignores it.
// The test does not run in parallel: a script written and started while another test forks
// fails with ETXTBSY.
// The fake git has no background child: a `kill $!` that lands between the child's fork and
// its exec of sleep is lost, and the sleep then holds the output pipe until WaitDelay (seen as
// a 10s step under load). A foreground loop of short sleeps lets the trap run within 0.1s.
func TestCancelledGitIsAskedToStopBeforeItIsKilled(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "got-term")
	script := filepath.Join(dir, "fakegit")
	body := "#!/bin/sh\ntrap 'echo term > \"" + marker + "\"; exit 143' TERM\n: > ready\nwhile :; do sleep 0.1; done\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil { //nolint:gosec // G306: a test script that has to be executable
		t.Fatal(err)
	}
	g := &gitops.Git{Bin: script, Dir: dir}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := g.Rev(ctx, dir, "main")
		done <- err
	}()
	// Wait for the shell to install its trap before testing cancellation.
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatalf("git exited before it was ready: %v", err)
		case <-ctx.Done():
			t.Fatal("git did not become ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
	start := time.Now()
	cancel()
	if err := <-done; err == nil {
		t.Fatal("a canceled git reported success")
	}
	if time.Since(start) > 8*time.Second {
		t.Errorf("the canceled git held the step for %v", time.Since(start))
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the canceled git was not asked to stop with SIGTERM: %v", err)
	}
}
