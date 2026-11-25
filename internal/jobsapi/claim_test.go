package jobsapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"pgregory.net/rapid"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// claimBook is a jobs API on a real book with one task per synthetic tenant.
type claimBook struct {
	pool  *pgxpool.Pool
	h     http.Handler
	tasks map[string]int64
}

func newClaimBook(t *testing.T, caps map[string]int, weights map[string]float64) *claimBook {
	t.Helper()
	db := pgtest.Start(t)
	b := &claimBook{pool: db.ServicePool(t), tasks: map[string]int64{}}
	q := bookdb.New(b.pool)
	for _, tenant := range []string{"example-solo", "example-team"} {
		task, err := q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: tenant, Repo: "example/app", Title: "Claims"})
		if err != nil {
			t.Fatal(err)
		}
		b.tasks[tenant] = task.ID
	}
	s := &jobsapi.Server{Pool: b.pool, WorkerTokenSHA256: jobsapi.HashToken(workerToken), ProviderCaps: caps, TenantWeights: weights}
	b.h = s.Handler()
	return b
}

func (b *claimBook) enqueue(t *testing.T, tenant, provider string) int64 {
	t.Helper()
	job, err := bookdb.New(b.pool).EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
		Tenant: tenant, TaskID: b.tasks[tenant], Role: "builder", Provider: provider, Payload: []byte(`{"prompt":"do it"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return job.ID
}

// claim returns the claimed job's id, zero when nothing was leased.
func (b *claimBook) claim(t *testing.T) int64 {
	t.Helper()
	rec := call(t, b.h, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: "w1"})
	var resp jobsapi.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("claim: %d %s %v", rec.Code, rec.Body, err)
	}
	if resp.JobID == "" {
		return 0
	}
	id, err := strconv.ParseInt(resp.JobID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (b *claimBook) claims(t *testing.T, n int) []int64 {
	t.Helper()
	got := make([]int64, n)
	for i := range got {
		got[i] = b.claim(t)
	}
	return got
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestAClaimSharesWorkersAcrossTenants: a long queue of one tenant does not keep another
// tenant's job waiting, and budget.weight gives a tenant a larger share (T3).
func TestAClaimSharesWorkersAcrossTenants(t *testing.T) {
	t.Run("equal weights", func(t *testing.T) {
		b := newClaimBook(t, nil, nil)
		s1, s2, s3 := b.enqueue(t, "example-solo", "p"), b.enqueue(t, "example-solo", "p"), b.enqueue(t, "example-solo", "p")
		t1 := b.enqueue(t, "example-team", "p")
		if got, want := b.claims(t, 5), []int64{s1, t1, s2, s3, 0}; !equalIDs(got, want) {
			t.Fatalf("claims %v, want %v", got, want)
		}
	})
	t.Run("a heavier tenant gets more", func(t *testing.T) {
		b := newClaimBook(t, nil, map[string]float64{"example-solo": 2})
		s1, s2, s3 := b.enqueue(t, "example-solo", "p"), b.enqueue(t, "example-solo", "p"), b.enqueue(t, "example-solo", "p")
		t1, t2 := b.enqueue(t, "example-team", "p"), b.enqueue(t, "example-team", "p")
		// Running per weight: solo 0, 0.5, 0.5, 1; team 0, 0, 1, 1. A tie goes to the older job.
		if got, want := b.claims(t, 5), []int64{s1, t1, s2, s3, t2}; !equalIDs(got, want) {
			t.Fatalf("claims %v, want %v", got, want)
		}
	})
}

// TestAClaimKeepsAProviderCap: a provider at its concurrency is skipped, the others are not,
// and parallel claims never pass the cap (T3).
func TestAClaimKeepsAProviderCap(t *testing.T) {
	t.Run("skipped, not blocking", func(t *testing.T) {
		b := newClaimBook(t, map[string]int{"sub": 1}, nil)
		sub1 := b.enqueue(t, "example-solo", "sub")
		b.enqueue(t, "example-team", "sub")
		api := b.enqueue(t, "example-solo", "api")
		if got, want := b.claims(t, 3), []int64{sub1, api, 0}; !equalIDs(got, want) {
			t.Fatalf("claims %v, want %v", got, want)
		}
	})
	t.Run("parallel claims", func(t *testing.T) {
		b := newClaimBook(t, map[string]int{"sub": 2}, nil)
		for i := range 10 {
			b.enqueue(t, []string{"example-solo", "example-team"}[i%2], "sub")
		}
		var wg sync.WaitGroup
		got := make([]int64, 10)
		for i := range got {
			wg.Go(func() { got[i] = b.claim(t) })
		}
		wg.Wait()
		leased := 0
		for _, id := range got {
			if id != 0 {
				leased++
			}
		}
		if leased != 2 {
			t.Fatalf("%d jobs leased under a cap of 2: %v", leased, got)
		}
	})
}

type bookJob struct {
	id               int64
	tenant, provider string
	state            string
}

func (b *claimBook) jobs(t *rapid.T) []bookJob {
	rows, err := b.pool.Query(t.Context(), `SELECT id, tenant, provider, state FROM book.jobs WHERE state IN ('queued', 'running') ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []bookJob
	for rows.Next() {
		var j bookJob
		if err := rows.Scan(&j.id, &j.tenant, &j.provider, &j.state); err != nil {
			t.Fatal(err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// expectedClaim is the job a claim must take: among queued jobs whose provider has room, the
// one of the tenant with the fewest running jobs per weight, then the oldest.
func expectedClaim(jobs []bookJob, caps map[string]int, weights map[string]float64) int64 {
	byProvider, byTenant := map[string]int{}, map[string]int{}
	for _, j := range jobs {
		if j.state == "running" {
			byProvider[j.provider]++
			byTenant[j.tenant]++
		}
	}
	var best int64
	bestLoad := 0.0
	for _, j := range jobs {
		if j.state != "queued" || (caps[j.provider] > 0 && byProvider[j.provider] >= caps[j.provider]) {
			continue
		}
		w := weights[j.tenant]
		if w == 0 {
			w = 1
		}
		load := float64(byTenant[j.tenant]) / w
		if best == 0 || load < bestLoad {
			best, bestLoad = j.id, load
		}
	}
	return best
}

// TestClaimProperty runs random enqueues, claims and completions: every claim takes the job
// the rule above names, and no provider ever runs more than its cap (T2, rapid on the book).
func TestClaimProperty(t *testing.T) {
	caps := map[string]int{"sub": 1, "api": 2, "free": 0}
	weights := map[string]float64{"example-solo": 2}
	b := newClaimBook(t, caps, weights)
	rapid.Check(t, func(rt *rapid.T) {
		defer func() {
			if _, err := b.pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'canceled' WHERE state IN ('queued', 'running')`); err != nil {
				rt.Fatal(err)
			}
		}()
		for range rapid.IntRange(1, 25).Draw(rt, "steps") {
			switch rapid.IntRange(0, 2).Draw(rt, "op") {
			case 0:
				b.enqueue(t, rapid.SampledFrom([]string{"example-solo", "example-team"}).Draw(rt, "tenant"),
					rapid.SampledFrom([]string{"sub", "api", "free"}).Draw(rt, "provider"))
			case 1:
				want := expectedClaim(b.jobs(rt), caps, weights)
				if got := b.claim(t); got != want {
					rt.Fatalf("claimed %d, want %d", got, want)
				}
			case 2:
				if _, err := b.pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'done' WHERE id = (
					SELECT id FROM book.jobs WHERE state = 'running' ORDER BY random() LIMIT 1)`); err != nil {
					rt.Fatal(err)
				}
			}
			running := map[string]int{}
			for _, j := range b.jobs(rt) {
				if j.state == "running" {
					running[j.provider]++
				}
			}
			for p, n := range running {
				if caps[p] > 0 && n > caps[p] {
					rt.Fatalf("provider %s runs %d jobs over its cap %d", p, n, caps[p])
				}
			}
		}
	})
}
