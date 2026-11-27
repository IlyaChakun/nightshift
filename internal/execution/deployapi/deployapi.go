// Package deployapi is the deploy target contract (design section 17, "Deploy targets", ADR
// 0014): the Deployer interface the conductor drives and the values it exchanges with every
// adapter and with the deployer service. It holds no behavior.
package deployapi

import (
	"context"
	"errors"
)

// Deployer is a deploy target as the conductor uses it (design section 17, "Deploy targets",
// ADR 0014): a preview or production environment on one platform. Every operation is
// idempotent and keyed by tenant, task and step, so after a restart the conductor asks Status
// and continues. An adapter that cannot do an operation returns ErrUnsupported.
type Deployer interface {
	// Up brings an environment up at a revision and returns where it answers.
	Up(ctx context.Context, e EnvRef, rev Revision) (Endpoint, error)
	// Down removes an environment; an environment that is already absent is not an error.
	Down(ctx context.Context, e EnvRef) error
	// Release makes rev the production revision and returns the one it replaced. It must be safe
	// to call again for the same rev after a crash and then return the same Previous: the
	// conductor's journal repeats a Release whose answer it did not record. When rev was started
	// but did not become healthy, it returns the Released it knows (Previous set) together with
	// an error that wraps ErrRolloutFailed, so the conductor can roll back.
	Release(ctx context.Context, rev Revision) (Released, error)
	// Smoke checks an environment that is up: healthchecks first, then the project's own check.
	Smoke(ctx context.Context, e EnvRef) (SmokeResult, error)
	// Rollback makes to the production revision again.
	Rollback(ctx context.Context, to Revision) error
	Status(ctx context.Context, e EnvRef) (EnvStatus, error)
}

// ErrUnsupported is returned by an adapter for an operation its platform does not have.
var ErrUnsupported = errors.New("deploy operation is not supported by this adapter")

// ErrRolloutFailed means the new revision was started and did not come up healthy: a defect of
// the revision, not of the platform. The previous revision is still the way back.
var ErrRolloutFailed = errors.New("the new revision did not become healthy")

// ErrRefused means the platform refused the change itself, for example a recipe that breaks a
// rule of the adapter: a defect of the code under test, not of the platform, so the task goes
// back to Builder and nothing is retried.
var ErrRefused = errors.New("the deploy target refused the change")

// Revision is a commit: of the task branch for a preview, of main for a release.
type Revision string

// EnvRef names one environment. Task is empty for production.
type EnvRef struct {
	Tenant string `json:"tenant"`
	Task   string `json:"task,omitempty"`
}

// Endpoint is where an environment answers.
type Endpoint struct {
	URL string `json:"url"`
}

// Released reports a release: the revision that was live before it, for rollback.
type Released struct {
	Revision Revision `json:"revision"`
	Previous Revision `json:"previous,omitempty"`
}

// SmokeResult is the outcome of a smoke check; Detail is the short reason when it failed.
type SmokeResult struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// EnvState is the health of an environment.
type EnvState string

// The states of an environment.
const (
	EnvAbsent    EnvState = "absent"
	EnvStarting  EnvState = "starting"
	EnvHealthy   EnvState = "healthy"
	EnvUnhealthy EnvState = "unhealthy"
)

// EnvStatus is what Status reports.
type EnvStatus struct {
	State    EnvState `json:"state"`
	Revision Revision `json:"revision,omitempty"`
	// Previous is the revision this one replaced, when the platform remembers it.
	Previous Revision `json:"previous,omitempty"`
}
