package runner

import (
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
)

// Without an exporter the agent still gets the task's trace context; with one it gets the
// span of its turn, which is in the same trace.
func TestJobSpanStaysInTheTaskTrace(t *testing.T) {
	t.Parallel()
	const tp = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx, span := jobSpan(t.Context(), "1", jobsapi.Job{Role: "builder", Traceparent: tp})
	defer span.End()
	if got := trace.SpanContextFromContext(ctx).TraceID().String(); got != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace id %q", got)
	}
	if got := traceparent(ctx, "fallback"); got != tp {
		t.Errorf("traceparent %q", got)
	}
	if got := traceparent(t.Context(), "fallback"); got != "fallback" {
		t.Errorf("without a span: %q", got)
	}
}
