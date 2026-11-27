package dispatch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/doors"

	"github.com/jackc/pgx/v5/pgtype"
	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/golden"
)

const scenarioDir = "../../../testdata/scenarios"

// TestScenarios replays every testdata/scenarios/*.yaml (design section 20, T4): the chat steps
// drive the Inbox, the agent steps complete jobs like the fake harness would, and the twins
// stand in for GitHub. The book's events are compared with a golden file where a scenario names one.
func TestScenarios(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(scenarioDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios: %v", err)
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".yaml"), func(t *testing.T) {
			raw, err := os.ReadFile(file) //nolint:gosec // G304: the path comes from a glob of the scenario directory
			if err != nil {
				t.Fatal(err)
			}
			var sc struct {
				Given struct {
					ReviewMode      string          `yaml:"review_mode"`
					WithoutVerifier bool            `yaml:"without_verifier"`
					Deploy          *scenarioDeploy `yaml:"deploy"`
				} `yaml:"given"`
				Steps []map[string]yaml.Node `yaml:"steps"`
			}
			if err := yaml.Unmarshal(raw, &sc); err != nil {
				t.Fatal(err)
			}
			r := scenarioRig(t, sc.Given.ReviewMode, sc.Given.WithoutVerifier, sc.Given.Deploy)
			for i, step := range sc.Steps {
				for kind, node := range step {
					r.play(t, fmt.Sprintf("step %d (%s)", i+1, kind), kind, node)
				}
			}
		})
	}
}

// scenarioRig sets up the tenant a scenario's given block describes.
func scenarioRig(t *testing.T, reviewMode string, withoutVerifier bool, d *scenarioDeploy) *rig {
	t.Helper()
	r, _ := leadRig(t)
	r.withGate(config.Review{Mode: reviewMode}, "solo")
	if !withoutVerifier {
		r.withVerifier()
	}
	if d != nil {
		r.withDeploy(*d)
	}
	return r
}

// play runs one scenario step.
func (r *rig) play(t *testing.T, name, kind string, node yaml.Node) {
	t.Helper()
	ctx := t.Context()
	decode := func(v any) {
		t.Helper()
		if err := node.Decode(v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	switch kind {
	case "chat":
		var s struct{ Text string }
		decode(&s)
		if s.Text != r.task.Title { // the first message is the idea itself
			r.say("owner_message", s.Text)
		}
		r.advance()
	case "agent":
		r.agentStep(t, name, node)
	case "click":
		var c struct{ Card, Action string }
		decode(&c)
		r.click(ctx, t, c.Card, c.Action)
	case "ci":
		var c struct{ Status string }
		decode(&c)
		r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: c.Status})
		r.advance()
	case "github_review":
		var g struct{ State string }
		decode(&g)
		r.advance() // the task must have reached human review
		r.twin.AddReview("example/app", 1, fake.Review{User: "owner", State: g.State, CommitSHA: r.task.HeadSha.String})
		r.advance()
	case "expect":
		r.expect(t, name, node)
	default:
		t.Fatalf("%s: unknown step %q", name, kind)
	}
}

func (r *rig) click(ctx context.Context, t *testing.T, card, action string) {
	t.Helper()
	if card != "spec" { // the deploy card: the train merges on the next tick
		r.advance()
		return
	}
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	inbox := &doors.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	ev := chatapi.ChatEvent{Kind: chatapi.ChatButton, User: "U1", Action: action, TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}
	if err := inbox.Handle(ctx, r.tenant, ev); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
}

func (r *rig) agentStep(t *testing.T, name string, node yaml.Node) {
	t.Helper()
	var a struct {
		Role   string
		Result yaml.Node
		Patch  string
	}
	if err := node.Decode(&a); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	result := r.resultJSON(t, name, a.Result)
	switch a.Role {
	case "lead":
		r.advance() // grilling: the Lead job is queued
		r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + result + `}`)
	case "builder":
		var patch []byte
		if a.Patch != "" {
			var err error
			if patch, err = os.ReadFile(filepath.Join(scenarioDir, a.Patch)); err != nil {
				t.Fatal(err)
			}
		}
		r.finish(patch)
		if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET result = $2 WHERE id = $1`, r.lastJob().ID, []byte(result)); err != nil {
			t.Fatal(err)
		}
	case "verifier":
		r.advance() // review: the Verifier job is queued
		r.verdict(result)
	default:
		t.Fatalf("%s: unknown role %q", name, a.Role)
	}
	r.advance()
}

// resultJSON reads a result given as a path to a JSON file or as an inline mapping.
func (r *rig) resultJSON(t *testing.T, name string, n yaml.Node) string {
	t.Helper()
	if n.Kind == yaml.ScalarNode {
		b, err := os.ReadFile(filepath.Join(scenarioDir, n.Value))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return string(b)
	}
	var v map[string]any
	if err := n.Decode(&v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (r *rig) expect(t *testing.T, name string, n yaml.Node) {
	t.Helper()
	var e struct {
		State        string
		GithubStatus string `yaml:"github_status"`
		Events       string
		// Preview is the task's preview after the conductor's teardown sweep: healthy or absent.
		Preview string
		// Production is the live revision: "merge" (the task's merge commit) or "previous".
		Production string
	}
	if err := n.Decode(&e); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	// Settle: the conductor moves one state per tick, and the scenario names the resting state.
	for range 8 {
		before := r.task.State
		r.advance()
		if r.task.State == before {
			break
		}
	}
	if e.State != "" && r.task.State != e.State {
		t.Fatalf("%s: the task is in %s, want %s", name, r.task.State, e.State)
	}
	r.expectDeploy(t, name, e.Preview, e.Production)
	if e.GithubStatus != "" {
		status, want, _ := strings.Cut(e.GithubStatus, "=")
		if got := r.twin.Status("example/app", r.task.HeadSha.String, status); got != want {
			t.Fatalf("%s: status %s is %q, want %q", name, status, got, want)
		}
	}
	if e.Events != "" {
		events, err := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, ev := range events {
			fmt.Fprintf(&b, "%s %s\n", ev.Kind, ev.Actor)
		}
		golden.Assert(t, filepath.Join(scenarioDir, "..", e.Events), []byte(b.String()))
	}
}

func taskID(id int64) pgtype.Int8 { return pgtype.Int8{Int64: id, Valid: true} }
