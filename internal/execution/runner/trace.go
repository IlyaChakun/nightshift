package runner

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// tracer is the source of the runner's spans.
func tracer() trace.Tracer { return otel.Tracer("nightshift/runner") }

// jobSpan starts the span of a job inside the task's trace, which the job carries.
func jobSpan(ctx context.Context, jobID string, job jobsapi.Job) (context.Context, trace.Span) {
	ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": job.Traceparent})
	return tracer().Start(ctx, "job."+job.Role, trace.WithAttributes( //nolint:spancheck // the caller ends it
		attribute.String("nightshift.tenant", job.Tenant), attribute.String("nightshift.task", job.TaskID),
		attribute.String("nightshift.job", jobID), attribute.String("nightshift.provider", job.Provider)))
}

// traceparent renders the span in ctx for the agent, or fallback without one.
func traceparent(ctx context.Context, fallback string) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	if tp := carrier["traceparent"]; tp != "" {
		return tp
	}
	return fallback
}
