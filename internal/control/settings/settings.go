// Package settings holds the operational switches in the book (design section 15): pause,
// review mode, SLA auto-merge, parallelism and the weekly share. Every change is validated
// and written together with an event that names its author.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// Keys. Pause is an instance setting (Instance as the tenant); the others are per tenant.
const (
	Instance      = ""
	Paused        = "paused"
	ReviewMode    = "review_mode"
	SLAAutoMerge  = "sla_automerge"
	Parallelism   = "parallelism"
	WeeklyShare   = "weekly_share"
	eventSettings = "settings.changed"
)

// ErrInvalid means the key is unknown or the value does not fit it.
var ErrInvalid = errors.New("settings: invalid setting")

// Store reads and writes settings.
type Store struct{ Pool *pgxpool.Pool }

// validate checks a value against its key and returns it as JSON.
func validate(tenant, key string, value any) ([]byte, error) {
	ok := false
	switch key {
	case Paused:
		_, ok = value.(bool)
		ok = ok && tenant == Instance
	case SLAAutoMerge:
		_, ok = value.(bool)
		ok = ok && tenant != Instance
	case ReviewMode:
		s, isStr := value.(string)
		ok = isStr && tenant != Instance && (s == "all" || s == "blast_radius" || s == "off")
	case Parallelism:
		n, isInt := value.(int)
		ok = isInt && n >= 1 && n <= 64 && tenant != Instance
	case WeeklyShare:
		f, isFloat := value.(float64)
		ok = isFloat && f >= 0 && f <= 1 && tenant != Instance
	}
	if !ok {
		return nil, fmt.Errorf("%w: %q = %v for tenant %q", ErrInvalid, key, value, tenant)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	return b, nil
}

// Set validates and stores a setting and appends the event that records who changed it and
// what it was before, in one transaction.
func (s *Store) Set(ctx context.Context, tenant, key string, value any, actor string) error {
	raw, err := validate(tenant, key, value)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		var before json.RawMessage
		if old, err := q.GetSetting(ctx, bookdb.GetSettingParams{Tenant: tenant, Key: key}); err == nil {
			before = old.Value
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("settings: read %s: %w", key, err)
		}
		if _, err := q.PutSetting(ctx, bookdb.PutSettingParams{Tenant: tenant, Key: key, Value: raw, UpdatedBy: actor}); err != nil {
			return fmt.Errorf("settings: write %s: %w", key, err)
		}
		payload, err := json.Marshal(map[string]any{"key": key, "from": before, "to": json.RawMessage(raw)})
		if err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		if _, err := q.AppendEvent(ctx, bookdb.AppendEventParams{Tenant: tenant, Kind: eventSettings, Actor: actor, Payload: payload}); err != nil {
			return fmt.Errorf("settings: event: %w", err)
		}
		return nil
	})
}

// Get returns the stored value, or false when the setting was never set.
func (s *Store) Get(ctx context.Context, tenant, key string, into any) (bool, error) {
	row, err := bookdb.New(s.Pool).GetSetting(ctx, bookdb.GetSettingParams{Tenant: tenant, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("settings: read %s: %w", key, err)
	}
	if err := json.Unmarshal(row.Value, into); err != nil {
		return false, fmt.Errorf("settings: decode %s: %w", key, err)
	}
	return true, nil
}

// IsPaused reports whether the factory is paused: the dispatcher, the job claims and the
// merge trains stop starting new work.
func (s *Store) IsPaused(ctx context.Context) (bool, error) {
	var paused bool
	if _, err := s.Get(ctx, Instance, Paused, &paused); err != nil {
		return false, err
	}
	return paused, nil
}
