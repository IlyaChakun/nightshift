package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

type recorder struct {
	mu   sync.Mutex
	sent []dispatch.Message
	fail error
}

func (r *recorder) Send(_ context.Context, m dispatch.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	r.sent = append(r.sent, m)
	return nil
}

func TestOutbox(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx := t.Context()
	chat := &recorder{}
	o := &dispatch.Outbox{
		Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat},
		Mask: dispatch.NewMasker("instance-secret-value"),
	}
	q := bookdb.New(pool)
	enqueue := func(key, text string) {
		t.Helper()
		if err := o.Enqueue(ctx, q, "example-solo", 0, "chat", key, []byte(fmt.Sprintf(`{"text":%q}`, text))); err != nil {
			t.Fatal(err)
		}
	}

	enqueue("k1", "spec ready, token instance-secret-value")
	enqueue("k1", "spec ready, token instance-secret-value") // a repeat is dropped
	if n, err := o.Flush(ctx); err != nil || n != 1 {
		t.Fatalf("flush: %d, %v", n, err)
	}
	if len(chat.sent) != 1 || strings.Contains(string(chat.sent[0].Payload), "instance-secret-value") {
		t.Fatalf("sent: %+v", chat.sent)
	}

	chat.fail = errors.New("chat API: 429")
	enqueue("k2", "retry me")
	if n, err := o.Flush(ctx); err != nil || n != 0 {
		t.Fatalf("failing flush: %d, %v", n, err)
	}
	if n, _ := o.Flush(ctx); n != 0 {
		t.Fatal("a message in backoff was sent again")
	}
	chat.fail = fmt.Errorf("channel archived: %w", dispatch.ErrPermanent)
	enqueue("k3", "never delivered")
	if _, err := o.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.Owner.QueryRowContext(ctx, `SELECT status FROM book.outbox WHERE idempotency_key = 'k3'`).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("permanent failure: status %q, %v", status, err)
	}
}

func TestRiverRunsTheOutbox(t *testing.T) {
	db := pgtest.Start(t)
	ctx := t.Context()
	if err := dispatch.Migrate(ctx, db.Owner); err != nil {
		t.Fatal(err)
	}
	if err := dispatch.Migrate(ctx, db.Owner); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	pool := db.ServicePool(t)
	chat := &recorder{}
	o := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{"chat": chat}, Mask: dispatch.NewMasker()}
	if err := o.Enqueue(ctx, bookdb.New(pool), "example-solo", 0, "chat", "hello", []byte(`{"text":"hello"}`)); err != nil {
		t.Fatal(err)
	}
	client, err := dispatch.NewClient(pool, o, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Stop(ctx) }()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		chat.mu.Lock()
		n := len(chat.sent)
		chat.mu.Unlock()
		if n == 1 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("River did not deliver the outbox message")
}

// cancelingSender cancels the flush context while it sends, like a shutdown that
// arrives between the send and the bookkeeping.
type cancelingSender struct {
	cancel context.CancelFunc
	sent   int
}

func (c *cancelingSender) Send(context.Context, dispatch.Message) error {
	c.sent++
	c.cancel()
	return nil
}

// TestOutboxMarksSentAfterShutdown is the regression test for a reply that was sent twice
// after a restart because shutdown canceled the write that marked it delivered.
func TestOutboxMarksSentAfterShutdown(t *testing.T) {
	db := pgtest.Start(t)
	if err := dispatch.Migrate(t.Context(), db.Owner); err != nil {
		t.Fatal(err)
	}
	pool := db.ServicePool(t)
	ctx, cancel := context.WithCancel(t.Context())
	s := &cancelingSender{cancel: cancel}
	o := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{"chat": s}, Mask: dispatch.NewMasker()}
	if err := o.Enqueue(t.Context(), bookdb.New(pool), "example-solo", 0, "chat", "k1", []byte(`{"text":"once"}`)); err != nil {
		t.Fatal(err)
	}
	if n, err := o.Flush(ctx); err != nil || n != 1 {
		t.Fatalf("flush: %d, %v", n, err)
	}
	if n, err := o.Flush(t.Context()); err != nil || n != 0 || s.sent != 1 {
		t.Fatalf("after restart: flushed %d, sent %d, %v", n, s.sent, err)
	}
}
