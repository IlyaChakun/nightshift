package dispatch

import (
	"context"
	"crypto/sha256"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// taskContext puts the task's trace into ctx, so every job of a task lands in one trace
// (design section 27). The ids are derived from the task, so they survive restarts without
// being stored; the creation time keeps two books from sharing them.
func taskContext(ctx context.Context, task bookdb.BookTask) context.Context {
	sum := sha256.Sum256([]byte("nightshift-task:" + task.Tenant + ":" + strconv.FormatInt(task.ID, 10) +
		":" + task.CreatedAt.Time.UTC().Format(time.RFC3339Nano)))
	var traceID trace.TraceID
	var spanID trace.SpanID
	copy(traceID[:], sum[:])
	copy(spanID[:], sum[len(traceID):])
	return trace.ContextWithRemoteSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true,
	}))
}
