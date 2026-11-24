package jobsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the worker side of the jobs API.
type Client struct {
	BaseURL     string
	WorkerToken string
	HTTP        *http.Client // nil means http.DefaultClient
}

// Claim asks for the next job; an empty JobID in the result means none arrived within wait.
func (c *Client) Claim(ctx context.Context, workerID string, wait time.Duration) (ClaimResponse, error) {
	var resp ClaimResponse
	err := c.post(ctx, "/jobs/claim", c.WorkerToken, ClaimRequest{WorkerID: workerID, Wait: wait}, &resp)
	return resp, err
}

// Heartbeat extends the lease of a job.
func (c *Client) Heartbeat(ctx context.Context, jobID, token, stage string) error {
	return c.post(ctx, "/jobs/"+jobID+"/heartbeat", token, HeartbeatRequest{Stage: stage}, nil)
}

// Events appends progress events.
func (c *Client) Events(ctx context.Context, jobID, token string, events []Event) error {
	return c.post(ctx, "/jobs/"+jobID+"/events", token, EventsRequest{Events: events}, nil)
}

// Complete finishes a job.
func (c *Client) Complete(ctx context.Context, jobID, token string, req CompleteRequest) error {
	return c.post(ctx, "/jobs/"+jobID+"/complete", token, req, nil)
}

func (c *Client) post(ctx context.Context, path, token string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("jobsapi: encode %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("jobsapi: %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("jobsapi: %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return fmt.Errorf("jobsapi: %s: %w", path, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jobsapi: %s: status %d: %s", path, res.StatusCode, bytes.TrimSpace(data))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("jobsapi: decode %s: %w", path, err)
	}
	return nil
}
