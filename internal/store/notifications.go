package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// NotifyState is the firing state of one (target, metric) alert pair. It
// is what makes a threshold breach fire once rather than once per result,
// and what a recovery notification is derived from.
type NotifyState struct {
	TargetID    int64  `json:"target_id"`
	Metric      string `json:"metric"`
	LastFiredAt string `json:"last_fired_at"`
	Firing      bool   `json:"firing"`
}

// GetNotifyState returns the stored state for a (target, metric) pair.
// ok is false when the pair has never fired.
func (s *Store) GetNotifyState(ctx context.Context, targetID int64, metric string) (NotifyState, bool, error) {
	st := NotifyState{TargetID: targetID, Metric: metric}
	var last sql.NullString
	err := s.Read.QueryRowContext(ctx,
		`SELECT last_fired_at, firing FROM notification_state WHERE target_id=? AND metric=?`,
		targetID, metric).Scan(&last, &st.Firing)
	if errors.Is(err, sql.ErrNoRows) {
		return NotifyState{}, false, nil
	}
	if err != nil {
		return NotifyState{}, false, fmt.Errorf("get notification state: %w", err)
	}
	st.LastFiredAt = last.String
	return st, true, nil
}

// SetNotifyState upserts the firing state for a (target, metric) pair.
func (s *Store) SetNotifyState(ctx context.Context, st NotifyState) error {
	_, err := s.Write.ExecContext(ctx, `
		INSERT INTO notification_state(target_id,metric,last_fired_at,firing)
		VALUES(?,?,?,?)
		ON CONFLICT(target_id,metric) DO UPDATE SET
			last_fired_at=excluded.last_fired_at, firing=excluded.firing`,
		st.TargetID, st.Metric, nullString(st.LastFiredAt), st.Firing)
	if err != nil {
		return fmt.Errorf("set notification state: %w", err)
	}
	return nil
}

// ClearNotifyState removes the stored state for a (target, metric) pair.
func (s *Store) ClearNotifyState(ctx context.Context, targetID int64, metric string) error {
	_, err := s.Write.ExecContext(ctx,
		`DELETE FROM notification_state WHERE target_id=? AND metric=?`, targetID, metric)
	if err != nil {
		return fmt.Errorf("clear notification state: %w", err)
	}
	return nil
}
