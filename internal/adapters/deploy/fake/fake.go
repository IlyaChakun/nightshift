// Package fake is the deploy twin for tests: it keeps environments in memory and follows the
// contract of integration.Deployer (idempotent operations, the previous revision on release,
// status after a restart) so conductor logic is tested without Docker, a server or a cluster.
// One twin stands for one tenant's production stack; previews are keyed by EnvRef.
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Deployer implements integration.Deployer in memory.
type Deployer struct {
	mu       sync.Mutex
	envs     map[integration.EnvRef]integration.EnvStatus
	prod     integration.Revision
	released map[integration.Revision]integration.Released // replay of a repeated Release
	smoke    map[integration.EnvRef]integration.SmokeResult
	fail     map[string]error
	calls    []string
}

var _ integration.Deployer = (*Deployer)(nil)

// New returns a twin with nothing running.
func New() *Deployer {
	return &Deployer{
		envs:     map[integration.EnvRef]integration.EnvStatus{},
		released: map[integration.Revision]integration.Released{},
		smoke:    map[integration.EnvRef]integration.SmokeResult{},
		fail:     map[string]error{},
	}
}

// FailNext makes the next call of op ("up", "down", "release", "smoke", "rollback" or "status")
// return err once, as a real platform does when it is down.
func (d *Deployer) FailNext(op string, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fail[op] = err
}

// SetSmoke fixes the result Smoke returns for an environment; without it a healthy environment
// passes and any other fails.
func (d *Deployer) SetSmoke(e integration.EnvRef, r integration.SmokeResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.smoke[key(e)] = r
}

// SetState forces an environment into a state, for example "unhealthy" after a crash.
func (d *Deployer) SetState(e integration.EnvRef, s integration.EnvState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.envs[key(e)]
	st.State = s
	d.envs[key(e)] = st
}

// Calls returns the operations seen so far, as "op" or "op rev".
func (d *Deployer) Calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

// Production returns the revision that is live now, or "" before the first release.
func (d *Deployer) Production() integration.Revision {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.prod
}

// key maps every production reference to one entry: the twin stands for one tenant's stack.
func key(e integration.EnvRef) integration.EnvRef {
	if e.Task == "" {
		return integration.EnvRef{}
	}
	return e
}

// begin records a call and returns the injected failure for it, if any. Callers hold d.mu.
func (d *Deployer) begin(op string, rev integration.Revision) error {
	if rev != "" {
		d.calls = append(d.calls, op+" "+string(rev))
	} else {
		d.calls = append(d.calls, op)
	}
	if err := d.fail[op]; err != nil {
		delete(d.fail, op)
		return err
	}
	return nil
}

// Up starts the environment at rev. The same revision again is a no-op; another one replaces it.
func (d *Deployer) Up(_ context.Context, e integration.EnvRef, rev integration.Revision) (integration.Endpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("up", rev); err != nil {
		return integration.Endpoint{}, err
	}
	if e.Task == "" {
		return integration.Endpoint{}, fmt.Errorf("fake deploy: up of %q needs a task; production comes up by release", e.Tenant)
	}
	d.envs[key(e)] = integration.EnvStatus{State: integration.EnvHealthy, Revision: rev}
	return integration.Endpoint{URL: fmt.Sprintf("http://%s-%s.preview.invalid", e.Tenant, e.Task)}, nil
}

// Down removes a preview; an absent one is not an error.
func (d *Deployer) Down(_ context.Context, e integration.EnvRef) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("down", ""); err != nil {
		return err
	}
	if e.Task == "" {
		return fmt.Errorf("fake deploy: down of %q is not supported; production changes by release or rollback", e.Tenant)
	}
	delete(d.envs, key(e))
	delete(d.smoke, key(e))
	return nil
}

// Release makes rev live and reports the revision it replaced. A repeated release of the same
// revision returns the first answer, so a retry after a restart cannot lose the way back.
func (d *Deployer) Release(_ context.Context, rev integration.Revision) (integration.Released, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("release", rev); err != nil {
		return integration.Released{}, err
	}
	if r, ok := d.released[rev]; ok && d.prod == rev {
		return r, nil
	}
	r := integration.Released{Revision: rev, Previous: d.prod}
	d.released[rev] = r
	d.prod = rev
	d.envs[integration.EnvRef{}] = integration.EnvStatus{State: integration.EnvHealthy, Revision: rev}
	return r, nil
}

// Smoke reports the configured result for e, or health by default.
func (d *Deployer) Smoke(_ context.Context, e integration.EnvRef) (integration.SmokeResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("smoke", ""); err != nil {
		return integration.SmokeResult{}, err
	}
	if r, ok := d.smoke[key(e)]; ok {
		return r, nil
	}
	if st := d.envs[key(e)]; st.State != integration.EnvHealthy {
		return integration.SmokeResult{Detail: "environment is " + string(stateOrAbsent(st.State))}, nil
	}
	return integration.SmokeResult{OK: true}, nil
}

// Rollback makes to the live revision again.
func (d *Deployer) Rollback(_ context.Context, to integration.Revision) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("rollback", to); err != nil {
		return err
	}
	d.prod = to
	d.envs[integration.EnvRef{}] = integration.EnvStatus{State: integration.EnvHealthy, Revision: to}
	return nil
}

// Status reports an environment; production is the zero EnvRef.
func (d *Deployer) Status(_ context.Context, e integration.EnvRef) (integration.EnvStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.begin("status", ""); err != nil {
		return integration.EnvStatus{}, err
	}
	st, ok := d.envs[key(e)]
	if !ok {
		return integration.EnvStatus{State: integration.EnvAbsent}, nil
	}
	return st, nil
}

func stateOrAbsent(s integration.EnvState) integration.EnvState {
	if s == "" {
		return integration.EnvAbsent
	}
	return s
}
