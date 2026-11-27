package adapters

import (
	"errors"
	"fmt"

	"github.com/ilyachakun/nightshift/internal/adapters/tracker/fake"
	"github.com/ilyachakun/nightshift/internal/control/trackerapi"
)

// ErrNoTracker means the named tracker adapter is not built; the tenant then runs without a
// projection and the book stays its only view.
var ErrNoTracker = errors.New("adapters: no such tracker adapter")

// NewTracker builds a tracker adapter by name. A real tracker (Linear, Jira) arrives with the
// first tenant that uses one.
func NewTracker(name string) (trackerapi.Tracker, error) {
	if name == "fake" {
		return fake.New(nil), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoTracker, name)
}
