// Package remote is the core's side of the deployer service (design section 17, "Where it
// runs"): a Deployer that sends each operation to the `deployer` service over HTTP. The service
// holds the Docker engine; the core never does. Spec carries what the service needs from the
// tenant's project.yaml on main, so the caller of the service decides nothing about the recipe.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Spec is the compose-local configuration of a tenant, read from main.
type Spec struct {
	File   string  `json:"file"`
	Entry  string  `json:"entry"`
	CPUs   float64 `json:"cpus,omitempty"`
	Memory string  `json:"memory,omitempty"`
	Smoke  string  `json:"smoke,omitempty"`
	// Base is the branch the recipe and the smoke check are read from.
	Base string `json:"base"`
}

// Request is one operation. Env and Rev apply as the operation needs them.
type Request struct {
	Tenant string               `json:"tenant"`
	Repo   string               `json:"repo"`
	Spec   Spec                 `json:"spec"`
	Env    integration.EnvRef   `json:"env"`
	Rev    integration.Revision `json:"rev"`
}

// Response is the answer; exactly one result field is set on success.
type Response struct {
	Endpoint *integration.Endpoint    `json:"endpoint,omitempty"`
	Released *integration.Released    `json:"released,omitempty"`
	Smoke    *integration.SmokeResult `json:"smoke,omitempty"`
	Status   *integration.EnvStatus   `json:"status,omitempty"`
	Error    string                   `json:"error,omitempty"`
	// Kind classifies an error: "refused" (a defect of the change: the recipe breaks a rule),
	// "rollout_failed" (the new revision did not become healthy; Released says where to go back
	// to), "unsupported", or empty (the platform failed).
	Kind string `json:"kind,omitempty"`
}

// Client implements integration.Deployer against the service.
type Client struct {
	URL, Token   string
	Tenant, Repo string
	Spec         Spec
	HTTP         *http.Client
}

var _ integration.Deployer = (*Client)(nil)

const maxResponse = 1 << 20

func (c *Client) call(ctx context.Context, op string, env integration.EnvRef, rev integration.Revision) (Response, error) {
	body, err := json.Marshal(Request{Tenant: c.Tenant, Repo: c.Repo, Spec: c.Spec, Env: env, Rev: rev})
	if err != nil {
		return Response{}, fmt.Errorf("deployer: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/v1/"+op, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("deployer: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Minute} // a build and a wait for healthchecks take minutes
	}
	res, err := hc.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("deployer: %s: %w", op, err)
	}
	defer res.Body.Close() //nolint:errcheck // a read-only body
	var out Response
	if err := json.NewDecoder(io.LimitReader(res.Body, maxResponse)).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("deployer: %s: status %d: %w", op, res.StatusCode, err)
	}
	if out.Error != "" {
		switch out.Kind {
		case "rollout_failed":
			return out, fmt.Errorf("%w: %s", integration.ErrRolloutFailed, out.Error)
		case "refused":
			return Response{}, fmt.Errorf("%w: %s", integration.ErrRefused, out.Error)
		case "unsupported":
			return Response{}, fmt.Errorf("%w: %s", integration.ErrUnsupported, out.Error)
		}
		return Response{}, fmt.Errorf("deployer: %s: %s", op, out.Error)
	}
	return out, nil
}

var errNoResult = errors.New("deployer: the service answered without a result")

// Up implements integration.Deployer.
func (c *Client) Up(ctx context.Context, e integration.EnvRef, rev integration.Revision) (integration.Endpoint, error) {
	r, err := c.call(ctx, "up", e, rev)
	if err != nil {
		return integration.Endpoint{}, err
	}
	if r.Endpoint == nil {
		return integration.Endpoint{}, errNoResult
	}
	return *r.Endpoint, nil
}

// Down implements integration.Deployer.
func (c *Client) Down(ctx context.Context, e integration.EnvRef) error {
	_, err := c.call(ctx, "down", e, "")
	return err
}

// Release implements integration.Deployer.
func (c *Client) Release(ctx context.Context, rev integration.Revision) (integration.Released, error) {
	r, err := c.call(ctx, "release", integration.EnvRef{Tenant: c.Tenant}, rev)
	if err != nil {
		if errors.Is(err, integration.ErrRolloutFailed) && r.Released != nil {
			return *r.Released, err
		}
		return integration.Released{}, err
	}
	if r.Released == nil {
		return integration.Released{}, errNoResult
	}
	return *r.Released, nil
}

// Smoke implements integration.Deployer.
func (c *Client) Smoke(ctx context.Context, e integration.EnvRef) (integration.SmokeResult, error) {
	r, err := c.call(ctx, "smoke", e, "")
	if err != nil {
		return integration.SmokeResult{}, err
	}
	if r.Smoke == nil {
		return integration.SmokeResult{}, errNoResult
	}
	return *r.Smoke, nil
}

// Rollback implements integration.Deployer.
func (c *Client) Rollback(ctx context.Context, to integration.Revision) error {
	_, err := c.call(ctx, "rollback", integration.EnvRef{Tenant: c.Tenant}, to)
	return err
}

// Status implements integration.Deployer.
func (c *Client) Status(ctx context.Context, e integration.EnvRef) (integration.EnvStatus, error) {
	r, err := c.call(ctx, "status", e, "")
	if err != nil {
		return integration.EnvStatus{}, err
	}
	if r.Status == nil {
		return integration.EnvStatus{}, errNoResult
	}
	return *r.Status, nil
}
