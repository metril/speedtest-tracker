// Package settings is a typed accessor over the settings table. Only the
// General section exists in milestone 1; Engines, Auth, Integrations and
// Notifications are added in later milestones.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/metril/speedtest-tracker/internal/store"
)

// General is the General settings section.
type General struct {
	BaseURL              string `json:"base_url"`
	Timezone             string `json:"timezone"`
	Units                string `json:"units"`
	LogLevel             string `json:"log_level"`
	RetentionDaysResults int    `json:"retention_days_results"`
	RetentionDaysRuns    int    `json:"retention_days_runs"`
}

// Keys of the General section.
const (
	KeyBaseURL              = "general.base_url"
	KeyTimezone             = "general.timezone"
	KeyUnits                = "general.units"
	KeyLogLevel             = "general.log_level"
	KeyRetentionDaysResults = "general.retention_days_results"
	KeyRetentionDaysRuns    = "general.retention_days_runs"
)

var defaults = map[string]any{
	KeyBaseURL:              "",
	KeyTimezone:             "UTC",
	KeyUnits:                "Mbps",
	KeyLogLevel:             "info",
	KeyRetentionDaysResults: 90,
	KeyRetentionDaysRuns:    30,
}

// Store reads and writes settings and notifies subscribers on change.
type Store struct {
	db *store.Store

	mu   sync.Mutex
	subs map[int]chan string
	next int
}

// New returns a Store and seeds any General key that is not yet present.
func New(ctx context.Context, db *store.Store) (*Store, error) {
	s := &Store{db: db, subs: map[int]chan string{}}
	for key, val := range defaults {
		encoded, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("encode default %s: %w", key, err)
		}
		if _, err := db.Write.ExecContext(ctx,
			`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`,
			key, string(encoded)); err != nil {
			return nil, fmt.Errorf("seed %s: %w", key, err)
		}
	}
	return s, nil
}

// Get returns the raw JSON value for key. ok is false if the key is unset.
func (s *Store) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	var v string
	err := s.db.Read.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get %s: %w", key, err)
	}
	return json.RawMessage(v), true, nil
}

// Set JSON-encodes value, stores it under key and notifies subscribers.
func (s *Store) Set(ctx context.Context, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	if _, err := s.db.Write.ExecContext(ctx, `
		INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET
			value=excluded.value,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		key, string(encoded)); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	s.notify(key)
	return nil
}

// General decodes the whole General section.
func (s *Store) General(ctx context.Context) (General, error) {
	g := General{}
	targets := map[string]any{
		KeyBaseURL:              &g.BaseURL,
		KeyTimezone:             &g.Timezone,
		KeyUnits:                &g.Units,
		KeyLogLevel:             &g.LogLevel,
		KeyRetentionDaysResults: &g.RetentionDaysResults,
		KeyRetentionDaysRuns:    &g.RetentionDaysRuns,
	}
	for key, dest := range targets {
		raw, ok, err := s.Get(ctx, key)
		if err != nil {
			return General{}, err
		}
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, dest); err != nil {
			return General{}, fmt.Errorf("decode %s: %w", key, err)
		}
	}
	return g, nil
}

// Subscribe returns a channel of changed keys and a cancel function. Sends
// are non-blocking: a subscriber that falls behind loses notifications
// rather than stalling the writer.
func (s *Store) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 16)
	s.mu.Lock()
	id := s.next
	s.next++
	s.subs[id] = ch
	s.mu.Unlock()

	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if c, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(c)
		}
	}
}

func (s *Store) notify(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- key:
		default:
		}
	}
}
