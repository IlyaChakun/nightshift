package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Service runs the chat routes and the outbox until its context ends. All state lives in
// the book, so a restarted service picks up where the last one stopped.
type Service struct {
	Pool        *pgxpool.Pool
	Routes      []Route
	Inbox       *Inbox
	Outbox      *Outbox
	FlushEvery  time.Duration
	StopTimeout time.Duration
	Log         *slog.Logger
}

// Run blocks until ctx ends, then lets running jobs finish within StopTimeout.
func (s *Service) Run(ctx context.Context) error {
	client, err := NewClient(s.Pool, s.Outbox, s.FlushEvery)
	if err != nil {
		return err
	}
	// River gets its own context so that a cancel means a graceful Stop below,
	// not an immediate hard stop.
	if err := client.Start(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("river: start: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, r := range s.Routes {
		events, err := r.Chat.Events(ctx)
		if err != nil {
			cancel()
			wg.Wait()
			_ = s.stop(ctx, client.Stop)
			return fmt.Errorf("chat events: %w", err)
		}
		wg.Go(func() {
			for ev := range events {
				s.handle(ctx, r, ev)
			}
		})
	}
	s.Log.Info("serving", "routes", len(s.Routes))
	<-ctx.Done()
	wg.Wait()
	return s.stop(ctx, client.Stop)
}

func (s *Service) stop(ctx context.Context, stop func(context.Context) error) error {
	timeout := s.StopTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := stop(stopCtx); err != nil {
		return fmt.Errorf("river: stop: %w", err)
	}
	s.Log.Info("stopped")
	return nil
}

func (s *Service) handle(ctx context.Context, r Route, ev integration.ChatEvent) {
	t, ok := r.TenantFor(ev.Thread.Channel)
	if !ok {
		s.Log.Warn("chat event in a channel of no tenant", "channel", ev.Thread.Channel)
		return
	}
	if err := s.Inbox.Handle(ctx, t, ev); err != nil {
		s.Log.Error("chat event failed", "tenant", t.Name, "error", err)
	}
}
