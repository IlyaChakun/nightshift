// Command spike-temporal is a throwaway experiment (ADR 0018, decision 15): one task flow up to
// OpenPR as a Temporal workflow whose activities call the existing Harness and PullRequests
// interfaces through the fake harness and a test double. Never merged into main.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/harness/fake"
	"github.com/ilyachakun/nightshift/internal/integration"
)

const queue = "nightshift-spike"

// --- test doubles -------------------------------------------------------------------------

// prDouble implements only OpenPR; calls count in a file so a repeat after a crash is visible.
type prDouble struct {
	integration.PullRequests
	dir string
}

func (p prDouble) OpenPR(_ context.Context, pr integration.OpenPR) (integration.PRRef, error) {
	f, err := os.OpenFile(filepath.Join(p.dir, "openpr.calls"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return integration.PRRef{}, err
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), pr.Title)
	return integration.PRRef{Repo: pr.Repo, Number: 1}, nil
}

// roleHarness gives a fake harness that holds only the first scenario step of one role, so an
// activity is repeatable in a fresh process (the stock fake keeps its cursor in memory).
func roleHarness(role string) (harness.Harness, error) {
	src := "testdata/scenarios/happy-path.yaml"
	raw, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	var sc map[string]any
	if err := yaml.Unmarshal(raw, &sc); err != nil {
		return nil, err
	}
	var keep []any
	for _, s := range sc["steps"].([]any) {
		if a, ok := s.(map[string]any)["agent"].(map[string]any); ok && a["role"] == role {
			keep = []any{s}
			break
		}
	}
	out, _ := yaml.Marshal(map[string]any{"given": sc["given"], "steps": keep})
	tmp := filepath.Join("testdata/scenarios", ".spike-"+role+".yaml")
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return nil, err
	}
	return fake.Load(tmp)
}

// --- activities ---------------------------------------------------------------------------

type Acts struct{ Dir string }

type RoleIn struct{ Role, Prompt string }
type RoleOut struct {
	Output  string
	Attempt int32
}

// flag reads a fault-injection knob from a file, so it can change while the worker runs.
func (a *Acts) flag(name string) int {
	b, err := os.ReadFile(filepath.Join(a.Dir, name))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// RunRole is one agent turn. It heartbeats like an adapter forwarding output events would.
func (a *Acts) RunRole(ctx context.Context, in RoleIn) (RoleOut, error) {
	attempt := activity.GetInfo(ctx).Attempt
	log.Printf("activity %s attempt %d pid %d", in.Role, attempt, os.Getpid())
	if in.Role == "builder" {
		if n := a.flag("fail-builder"); n >= int(attempt) {
			return RoleOut{}, fmt.Errorf("builder: provider overloaded (injected, attempt %d)", attempt)
		}
		for i := 0; i < a.flag("builder-seconds"); i++ { // a long agent run
			time.Sleep(time.Second)
			if a.flag("stall") == 0 {
				activity.RecordHeartbeat(ctx, i)
			}
		}
	}
	h, err := roleHarness(in.Role)
	if err != nil {
		return RoleOut{}, err
	}
	res, err := h.Run(ctx, harness.RunRequest{Role: harness.RoleConfig{Name: in.Role}, Prompt: in.Prompt, Workspace: a.Dir})
	if err != nil {
		return RoleOut{}, err
	}
	return RoleOut{Output: string(res.Output), Attempt: attempt}, nil
}

func (a *Acts) OpenPR(ctx context.Context, title string) (integration.PRRef, error) {
	return prDouble{dir: a.Dir}.OpenPR(ctx, integration.OpenPR{Repo: "example-solo/app", Head: "ns/task-1", Base: "main", Title: title, Draft: true})
}

// --- workflow -----------------------------------------------------------------------------

func TaskFlow(ctx workflow.Context, text string) (string, error) {
	opts := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:    15 * time.Second, // stall detector: no output heartbeat for 15 s
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, BackoffCoefficient: 2, MaximumAttempts: 4},
	}
	ctx = workflow.WithActivityOptions(ctx, opts)
	var a *Acts
	var lead, builder, verifier RoleOut
	if err := workflow.ExecuteActivity(ctx, a.RunRole, RoleIn{"lead", text}).Get(ctx, &lead); err != nil {
		return "", fmt.Errorf("lead: %w", err)
	}
	if err := workflow.ExecuteActivity(ctx, a.RunRole, RoleIn{"builder", lead.Output}).Get(ctx, &builder); err != nil {
		return "", fmt.Errorf("builder: %w", err)
	}
	if err := workflow.ExecuteActivity(ctx, a.RunRole, RoleIn{"verifier", builder.Output}).Get(ctx, &verifier); err != nil {
		return "", fmt.Errorf("verifier: %w", err)
	}
	var ref integration.PRRef
	if err := workflow.ExecuteActivity(ctx, a.OpenPR, "feat: "+text).Get(ctx, &ref); err != nil {
		return "", err
	}
	// Human wait: an outside event (button, merge outside the factory) arrives as a signal.
	var decision string
	sel := workflow.NewSelector(ctx)
	sel.AddReceive(workflow.GetSignalChannel(ctx, "human"), func(c workflow.ReceiveChannel, _ bool) { c.Receive(ctx, &decision) })
	sel.AddFuture(workflow.NewTimer(ctx, 24*time.Hour), func(workflow.Future) { decision = "timeout" })
	sel.Select(ctx)
	return fmt.Sprintf("PR #%d opened, human said %q", ref.Number, decision), nil
}

func main() {
	dir := os.Getenv("SPIKE_DIR")
	if dir == "" {
		log.Fatal("SPIKE_DIR required")
	}
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	switch os.Args[1] {
	case "worker":
		w := worker.New(c, queue, worker.Options{})
		w.RegisterWorkflow(TaskFlow)
		w.RegisterActivity(&Acts{Dir: dir})
		log.Fatal(w.Run(worker.InterruptCh()))
	case "start":
		run, err := c.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{ID: os.Args[2], TaskQueue: queue}, TaskFlow, "Add CSV export")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("started", run.GetID(), run.GetRunID())
	case "signal":
		if err := c.SignalWorkflow(context.Background(), os.Args[2], "", "human", "approved"); err != nil {
			log.Fatal(err)
		}
	case "result":
		var out string
		err := c.GetWorkflow(context.Background(), os.Args[2], "").Get(context.Background(), &out)
		var ae *temporal.ActivityError
		if errors.As(err, &ae) {
			fmt.Println("FAILED:", err)
			return
		}
		fmt.Println(out, err)
	}
}
