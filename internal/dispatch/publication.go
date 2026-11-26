package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
)

type recordedMove struct {
	Event           flow.Event `json:"event"`
	From            flow.State `json:"from"`
	To              flow.State `json:"to"`
	Version         int32      `json:"version"`
	ReviewSourceSHA string     `json:"review_source_sha"`
}

func readMove(e bookdb.BookEvent) recordedMove {
	var m recordedMove
	_ = json.Unmarshal(e.Payload, &m)
	return m
}

// publicationAllowed verifies the decision that admitted this task version to ready. A
// passing result alone does not prove the review policy admitted it. Human approval is
// rechecked on the forge, and SLA approval needs both its own event and a passing result.
func (c *Conductor) publicationAllowed(ctx context.Context, t config.Tenant, task bookdb.BookTask, ref integration.PRRef) (bool, error) {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("conductor: publication history for task %d: %w", task.ID, err)
	}
	approvedHead := task.HeadSha.String
	decision, sla := readinessDecision(events, flow.State(task.State), task.Version, approvedHead)
	if decision != flow.Accepted && decision != flow.Approve {
		return false, nil
	}
	approved, commented, requests, err := c.reviewDecisions(ctx, t, ref, approvedHead, approvedHead)
	if err != nil || len(requests) > 0 {
		return false, err
	}
	if decision == flow.Accepted {
		return c.hasPassingReview(ctx, task)
	}
	if approved || !sla || commented || c.profile(t) == flow.Team || t.Review.SLAAutomergeHours <= 0 || t.Review.Mode == "" {
		return approved, nil
	}
	return c.hasPassingReview(ctx, task)
}

func readinessDecision(events []bookdb.BookEvent, state flow.State, version int32, head string) (flow.Event, bool) {
	sla := false
	for _, e := range slices.Backward(events) {
		if e.Kind == eventSLAApproved && strings.HasPrefix(noteOf(e), head+": ") {
			sla = true
		}
		if e.Kind != "task.moved" {
			continue
		}
		m := readMove(e)
		switch {
		case m.Version != version || m.To != state:
			return "", false
		case state == flow.Merging && m.Event == flow.Merge && m.From == flow.Ready:
			state, version = flow.Ready, version-1
		case state == flow.Ready && m.Event == flow.Accepted && m.From == flow.Acceptance:
			return flow.Accepted, false
		case state == flow.Ready && m.Event == flow.Approve && m.From == flow.HumanReview:
			return flow.Approve, sla || e.Actor == slaReviewActor
		default:
			return "", false
		}
	}
	return "", false
}

// requireTrainReview recognizes a head the train has already rebased and published. An
// unrelated host head needs reconciliation rather than inheriting the train's checks.
func (c *Conductor) requireTrainReview(ctx context.Context, t config.Tenant, task bookdb.BookTask, head string) error {
	if head == "" {
		return fmt.Errorf("conductor: missing host head for task %d", task.ID)
	}
	source := ""
	if head != task.HeadSha.String {
		steps, err := bookdb.New(c.Pool).ListTrainSteps(ctx, task.ID)
		if err != nil {
			return fmt.Errorf("conductor: read train steps for task %d: %w", task.ID, err)
		}
		if !slices.ContainsFunc(steps, func(s bookdb.BookTrainStep) bool {
			return s.Step == train.StepRebase && s.State == "done" && s.Sha.String == head
		}) {
			return fmt.Errorf("conductor: host head %s for task %d has no completed train rebase", head, task.ID)
		}
		source = task.HeadSha.String
	}
	return c.openReview(ctx, t, task, head, source)
}

func (c *Conductor) previousReviewHead(ctx context.Context, task bookdb.BookTask) (string, error) {
	events, err := bookdb.New(c.Pool).ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return "", fmt.Errorf("conductor: review history: %w", err)
	}
	for _, event := range slices.Backward(events) {
		if event.Kind != "task.moved" {
			continue
		}
		move := readMove(event)
		if move.Version == task.Version && move.Event == flow.ReviewRequired {
			return move.ReviewSourceSHA, nil
		}
		break
	}
	return "", nil
}

func (c *Conductor) requireReview(ctx context.Context, t config.Tenant, task bookdb.BookTask, head string) error {
	return c.openReview(ctx, t, task, head, "")
}

func (c *Conductor) openReview(ctx context.Context, t config.Tenant, task bookdb.BookTask, head, equivalent string) error {
	var err error
	if equivalent == "" {
		task, err = c.move(ctx, t, task, flow.ReviewRequired, head)
	} else {
		task, err = c.Mover.Apply(ctx, flow.Move{
			TaskID: task.ID, Event: flow.ReviewRequired, Actor: "system", Profile: c.profile(t), HeadSHA: head, ReviewSourceSHA: equivalent,
		})
	}
	if err != nil {
		return err
	}
	return c.askHuman(ctx, t, task)
}
