package onboard

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// Check is one thing a person had to set up for an instance (docs/setup.md). Run returns what it found, or why it failed.
type Check struct {
	Name string
	Run  func(ctx context.Context) (string, error)
	Fix  string // what to do when it fails, naming the step of docs/setup.md
}

// checkTimeout bounds one check, so a host that never answers does not hang the report.
const checkTimeout = 20 * time.Second

// RunChecks runs the checks in order, writes one line for each and reports whether all passed.
func RunChecks(ctx context.Context, w io.Writer, checks []Check) bool {
	failed := 0
	for _, c := range checks {
		bounded, cancel := context.WithTimeout(ctx, checkTimeout)
		detail, err := c.Run(bounded)
		cancel()
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(w, "fail  %s: %s\n      fix: %s\n", c.Name, oneLine(err.Error()), c.Fix)
			continue
		}
		_, _ = fmt.Fprintf(w, "ok    %s: %s\n", c.Name, oneLine(detail))
	}
	_, _ = fmt.Fprintf(w, "%d checks, %d failed\n", len(checks), failed)
	return failed == 0
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
