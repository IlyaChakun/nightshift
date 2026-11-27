// Package jobserver serves the jobs API of package jobsapi over the book. It is the trusted
// half of the protocol: it claims rows, checks job tokens and validates every result before it
// writes. The worker imports only jobsapi and never reaches this package or the book.
package jobserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

const (
	defaultLease   = 5 * time.Minute
	maxClaimWait   = 30 * time.Second
	claimPollEvery = 500 * time.Millisecond
)

// Server serves the jobs API over the book. The worker token is compared by its sha256, so
// the process keeps no plaintext secret; a job token is random, stored only as a hash and
// dies with its lease.
type Server struct {
	Pool *pgxpool.Pool
	// WorkerTokenSHA256 is the hex sha256 of the worker token.
	WorkerTokenSHA256 string
	Lease             time.Duration // zero means five minutes
	Now               func() time.Time
	Log               *slog.Logger
	// ProviderCaps is each provider's concurrency and TenantWeights each tenant's
	// budget.weight; a claim keeps the first and shares workers by the second.
	// ClaimableProviders restricts leases to eligible providers in the loaded instance.
	// An empty map holds all jobs. A nil map leaves standalone servers unrestricted.
	ClaimableProviders map[string]BillingIdentity
	ProviderCaps       map[string]int
	TenantWeights      map[string]float64

	validatorOnce sync.Once
	validator     *config.Validator
}

// BillingIdentity is the part of a provider configuration that chooses where a job is billed.
type BillingIdentity struct {
	Harness string `json:"harness"`
	Auth    string `json:"auth"`
	KeyEnv  string `json:"key_env"`
	BaseURL string `json:"base_url"`
}

func (s *Server) schemas() *config.Validator {
	s.validatorOnce.Do(func() { s.validator = config.NewValidator() })
	return s.validator
}

// Handler returns the routes of the jobs API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(jobsapi.RouteClaim, s.claim)
	mux.HandleFunc(jobsapi.RouteHeartbeat, s.heartbeat)
	mux.HandleFunc(jobsapi.RouteEvents, s.events)
	mux.HandleFunc(jobsapi.RouteComplete, s.complete)
	return http.MaxBytesHandler(mux, jobsapi.MaxBody)
}

// HashToken returns the hex sha256 of a token, the form stored in the book.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) lease() time.Duration {
	if s.Lease > 0 {
		return s.Lease
	}
	return defaultLease
}

func (s *Server) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func bearer(r *http.Request) string {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return tok
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	s.log().ErrorContext(r.Context(), "jobs api", "path", r.URL.Path, "error", err)
	fail(w, http.StatusInternalServerError, "internal error")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad request body")
		return false
	}
	return true
}

func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	got := sha256.Sum256([]byte(bearer(r)))
	want, err := hex.DecodeString(s.WorkerTokenSHA256)
	if err != nil || len(want) != sha256.Size || subtle.ConstantTimeCompare(got[:], want) != 1 {
		fail(w, http.StatusUnauthorized, "bad worker token")
		return
	}
	var req jobsapi.ClaimRequest
	if !decode(w, r, &req) {
		return
	}
	if req.WorkerID == "" {
		fail(w, http.StatusBadRequest, "worker_id is required")
		return
	}
	wait := min(max(req.Wait, 0), maxClaimWait)
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(claimPollEvery)
	defer tick.Stop()
	for {
		resp, found, err := s.tryClaim(r.Context(), req.WorkerID)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		if found {
			reply(w, http.StatusOK, resp)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			reply(w, http.StatusOK, jobsapi.ClaimResponse{})
			return
		case <-tick.C:
		}
	}
}

func (s *Server) tryClaim(ctx context.Context, workerID string) (jobsapi.ClaimResponse, bool, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return jobsapi.ClaimResponse{}, false, fmt.Errorf("jobserver: token: %w", err)
	}
	token := hex.EncodeToString(raw)
	now := s.now()
	expires := now.Add(s.lease())
	row, err := s.claimRow(ctx, workerID, HashToken(token), now, expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobsapi.ClaimResponse{}, false, nil
	}
	if err != nil {
		return jobsapi.ClaimResponse{}, false, fmt.Errorf("jobserver: claim: %w", err)
	}
	var job jobsapi.Job
	if err := json.Unmarshal(row.Payload, &job); err != nil {
		return jobsapi.ClaimResponse{}, false, fmt.Errorf("jobserver: job %d payload: %w", row.ID, err)
	}
	job.Tenant, job.Role, job.Provider = row.Tenant, row.Role, row.Provider
	job.TaskID = strconv.FormatInt(row.TaskID, 10)
	job.Inputs = jobsapi.VisibleInputs(job.Role, job.Inputs)
	return jobsapi.ClaimResponse{JobID: strconv.FormatInt(row.ID, 10), JobToken: token, ExpiresAt: expires, Job: job}, true, nil
}

// maxAttempts is how many leases a job gets; a job whose last lease expired fails with
// lease_expired, and the conductor escalates it.
const maxAttempts = 3

// claimRow leases one job under the claim lock, so the provider caps hold when several
// workers claim at once.
func (s *Server) claimRow(ctx context.Context, workerID, tokenHash string, now, expires time.Time) (bookdb.BookJob, error) {
	var eligible []byte
	if s.ClaimableProviders != nil {
		var err error
		eligible, err = json.Marshal(s.ClaimableProviders)
		if err != nil {
			return bookdb.BookJob{}, fmt.Errorf("claimable providers: %w", err)
		}
	}
	caps, err := json.Marshal(s.ProviderCaps)
	if err != nil {
		return bookdb.BookJob{}, fmt.Errorf("provider caps: %w", err)
	}
	weights, err := json.Marshal(s.TenantWeights)
	if err != nil {
		return bookdb.BookJob{}, fmt.Errorf("tenant weights: %w", err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return bookdb.BookJob{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op after Commit
	q := bookdb.New(tx)
	if err := q.LockClaims(ctx); err != nil {
		return bookdb.BookJob{}, fmt.Errorf("lock: %w", err)
	}
	// A job whose worker died keeps its row running until its lease runs out; it is swept
	// here, before the caps count running jobs, or it would hold its provider's place and
	// its tenant's share for good.
	sweep := ts(now)
	if _, err := q.RequeueExpiredJobs(ctx, bookdb.RequeueExpiredJobsParams{Now: sweep, MaxAttempts: maxAttempts}); err != nil {
		return bookdb.BookJob{}, fmt.Errorf("requeue expired: %w", err)
	}
	if _, err := q.FailExpiredJobs(ctx, bookdb.FailExpiredJobsParams{Now: sweep, MaxAttempts: maxAttempts}); err != nil {
		return bookdb.BookJob{}, fmt.Errorf("fail expired: %w", err)
	}
	row, err := q.ClaimJob(ctx, bookdb.ClaimJobParams{
		WorkerID:    pgtype.Text{String: workerID, Valid: true},
		TokenSha256: pgtype.Text{String: tokenHash, Valid: true},
		LeaseUntil:  ts(expires), Now: ts(now),
		ProviderCaps: caps, TenantWeights: weights, ClaimableProviders: eligible,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing to lease, but the sweep above must still land: a job on its last attempt
		// fails now, not when some other job happens to be claimable.
		if cerr := tx.Commit(ctx); cerr != nil {
			return bookdb.BookJob{}, fmt.Errorf("commit sweep: %w", cerr)
		}
		return bookdb.BookJob{}, err
	}
	if err != nil {
		return bookdb.BookJob{}, err // the caller wraps it
	}
	return row, tx.Commit(ctx)
}

// leased finds the running job the request's token belongs to. It answers 401 itself.
func (s *Server) leased(w http.ResponseWriter, r *http.Request, q *bookdb.Queries) (bookdb.BookJob, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	tok := bearer(r)
	if err != nil || tok == "" {
		fail(w, http.StatusUnauthorized, "bad job token")
		return bookdb.BookJob{}, false
	}
	job, err := q.GetLeasedJob(r.Context(), bookdb.GetLeasedJobParams{
		ID: id, TokenSha256: pgtype.Text{String: HashToken(tok), Valid: true}, Now: ts(s.now()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusUnauthorized, "bad or expired job token")
		return bookdb.BookJob{}, false
	}
	if err != nil {
		s.internal(w, r, err)
		return bookdb.BookJob{}, false
	}
	return job, true
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var req jobsapi.HeartbeatRequest
	if !decode(w, r, &req) {
		return
	}
	q := bookdb.New(s.Pool)
	job, ok := s.leased(w, r, q)
	if !ok {
		return
	}
	now := s.now()
	until, err := q.ExtendLease(r.Context(), bookdb.ExtendLeaseParams{
		LeaseUntil: ts(now.Add(s.lease())), ID: job.ID, TokenSha256: job.TokenSha256, Now: ts(now),
	})
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if req.Stage != "" {
		if err := s.appendEvents(r.Context(), q, job, []jobsapi.Event{{At: now, Kind: "job.stage", Data: mustJSON(map[string]string{"stage": req.Stage})}}); err != nil {
			s.internal(w, r, err)
			return
		}
	}
	reply(w, http.StatusOK, map[string]time.Time{"expires_at": until.Time})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	var req jobsapi.EventsRequest
	if !decode(w, r, &req) {
		return
	}
	q := bookdb.New(s.Pool)
	job, ok := s.leased(w, r, q)
	if !ok {
		return
	}
	for _, e := range req.Events {
		if e.Kind == "" {
			fail(w, http.StatusBadRequest, "event kind is required")
			return
		}
	}
	if err := s.appendEvents(r.Context(), q, job, req.Events); err != nil {
		s.internal(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]int{"accepted": len(req.Events)})
}

// appendEvents writes worker events under the actor "worker:<role>"; the kind is prefixed so
// a worker cannot forge a core event such as a decision.
func (s *Server) appendEvents(ctx context.Context, q *bookdb.Queries, job bookdb.BookJob, events []jobsapi.Event) error {
	for _, e := range events {
		payload := e.Data
		if len(payload) == 0 {
			payload = json.RawMessage(`{}`)
		}
		_, err := q.AppendEvent(ctx, bookdb.AppendEventParams{
			Tenant: job.Tenant, TaskID: pgtype.Int8{Int64: job.TaskID, Valid: true},
			Kind: "worker." + strings.TrimPrefix(e.Kind, "worker."), Actor: "worker:" + job.Role,
			Payload: payload, TraceID: pgtype.Text{String: e.Trace, Valid: e.Trace != ""},
		})
		if err != nil {
			return fmt.Errorf("jobserver: append event: %w", err)
		}
	}
	return nil
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	var req jobsapi.CompleteRequest
	if !decode(w, r, &req) {
		return
	}
	switch req.ExitReason {
	case "done", "budget", "rate_limit", "error":
	default:
		fail(w, http.StatusBadRequest, "unknown exit_reason")
		return
	}
	for _, a := range req.Artifacts {
		if len(a.SHA256) != 64 || a.Path == "" || strings.HasPrefix(a.Path, "/") || strings.Contains(a.Path, "..") {
			fail(w, http.StatusBadRequest, "bad artifact "+a.Path)
			return
		}
	}
	if bearer(r) == "" {
		fail(w, http.StatusUnauthorized, "bad job token")
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		s.internal(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	q := bookdb.New(tx)
	job, ok := s.leased(w, r, q)
	if !ok {
		return
	}
	state := "failed"
	if req.ExitReason == "done" {
		state = "done"
	}
	usage, err := json.Marshal(req.Usage)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if _, err := q.CompleteJob(r.Context(), bookdb.CompleteJobParams{
		State: state, ExitReason: pgtype.Text{String: req.ExitReason, Valid: true},
		Result: req.Result, Usage: usage, Now: ts(s.now()), ID: job.ID, TokenSha256: job.TokenSha256,
	}); err != nil {
		s.internal(w, r, err)
		return
	}
	if err := s.recordOutputs(r.Context(), q, job, req, state); err != nil {
		s.internal(w, r, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		s.internal(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]string{"state": state})
}

// recordOutputs stores what a finished job produced: findings and notes (only from a job that
// completed), artifacts and the harness session.
func (s *Server) recordOutputs(ctx context.Context, q *bookdb.Queries, job bookdb.BookJob, req jobsapi.CompleteRequest, state string) error {
	if state == "done" {
		dropped, err := storeMemory(ctx, q, s.schemas(), job, req.Result, req.Findings)
		if err != nil {
			return err
		}
		if dropped > 0 {
			s.log().WarnContext(ctx, "dropped invalid findings or notes", "job", job.ID, "count", dropped)
		} else if coverage := findingCoverage(job.Role, req.CheckSuite); coverage != "" {
			if err := q.SetFindingCoverage(ctx, bookdb.SetFindingCoverageParams{
				JobID: job.ID, Coverage: pgtype.Text{String: coverage, Valid: true},
			}); err != nil {
				return fmt.Errorf("jobserver: record finding coverage: %w", err)
			}
		}
	}
	for _, a := range req.Artifacts {
		if err := q.AddArtifact(ctx, bookdb.AddArtifactParams{JobID: job.ID, Path: a.Path, Sha256: a.SHA256, Bytes: a.Bytes}); err != nil {
			return err
		}
	}
	if req.SessionID == "" {
		return nil
	}
	return q.SaveHarnessSession(ctx, bookdb.SaveHarnessSessionParams{
		TaskID: job.TaskID, Role: job.Role, Provider: job.Provider, SessionID: req.SessionID,
	})
}

func findingCoverage(role, suite string) string {
	if role == "verifier" {
		return role
	}
	hash := strings.TrimPrefix(suite, "partial:")
	if role == "builder" && len(hash) == sha256.Size*2 {
		if _, err := hex.DecodeString(hash); err == nil {
			return suite
		}
	}
	return ""
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
