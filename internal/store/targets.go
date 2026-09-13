package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Target is one configured speed-test target.
type Target struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	Engine     string          `json:"engine"`
	Enabled    bool            `json:"enabled"`
	Lane       string          `json:"lane"`
	Options    json.RawMessage `json:"options"`
	Thresholds json.RawMessage `json:"thresholds"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
}

const targetColumns = `id,name,engine,enabled,lane,options,thresholds,created_at,updated_at`

// rawOrEmpty normalises a nil/empty JSON document to "{}".
func rawOrEmpty(r json.RawMessage) string {
	if len(r) == 0 {
		return "{}"
	}
	return string(r)
}

func scanTarget(sc interface{ Scan(...any) error }) (*Target, error) {
	var t Target
	var options, thresh string
	if err := sc.Scan(&t.ID, &t.Name, &t.Engine, &t.Enabled, &t.Lane,
		&options, &thresh, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.Options = json.RawMessage(options)
	t.Thresholds = json.RawMessage(thresh)
	return &t, nil
}

// CreateTarget inserts t and returns the new row id.
func (s *Store) CreateTarget(ctx context.Context, t *Target) (int64, error) {
	res, err := s.Write.ExecContext(ctx,
		`INSERT INTO targets(name,engine,enabled,lane,options,thresholds) VALUES(?,?,?,?,?,?)`,
		t.Name, t.Engine, t.Enabled, t.Lane, rawOrEmpty(t.Options), rawOrEmpty(t.Thresholds))
	if err != nil {
		return 0, fmt.Errorf("insert target: %w", err)
	}
	return res.LastInsertId()
}

// GetTarget returns the target with the given id, or ErrNotFound.
func (s *Store) GetTarget(ctx context.Context, id int64) (*Target, error) {
	row := s.Read.QueryRowContext(ctx, `SELECT `+targetColumns+` FROM targets WHERE id=?`, id)
	t, err := scanTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get target %d: %w", id, err)
	}
	return t, nil
}

// ListTargets returns every target ordered by name.
func (s *Store) ListTargets(ctx context.Context) ([]Target, error) {
	rows, err := s.Read.QueryContext(ctx, `SELECT `+targetColumns+` FROM targets ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list targets: %w", err)
	}
	defer rows.Close()
	out := []Target{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListTargetsByIDs returns the named targets in the order they were
// requested; ids with no row are skipped.
func (s *Store) ListTargetsByIDs(ctx context.Context, ids []int64) ([]Target, error) {
	if len(ids) == 0 {
		return []Target{}, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `SELECT ` + targetColumns + ` FROM targets WHERE id IN (?` +
		strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := s.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list targets by ids: %w", err)
	}
	defer rows.Close()
	byID := map[int64]Target{}
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		byID[t.ID] = *t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Target, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// UpdateTarget writes every mutable field of t, or returns ErrNotFound.
func (s *Store) UpdateTarget(ctx context.Context, t *Target) error {
	res, err := s.Write.ExecContext(ctx, `
		UPDATE targets SET name=?,engine=?,enabled=?,lane=?,options=?,thresholds=?,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=?`,
		t.Name, t.Engine, t.Enabled, t.Lane, rawOrEmpty(t.Options), rawOrEmpty(t.Thresholds), t.ID)
	if err != nil {
		return fmt.Errorf("update target %d: %w", t.ID, err)
	}
	return requireAffected(res)
}

// DeleteTarget removes the target, or returns ErrNotFound.
func (s *Store) DeleteTarget(ctx context.Context, id int64) error {
	res, err := s.Write.ExecContext(ctx, `DELETE FROM targets WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete target %d: %w", id, err)
	}
	return requireAffected(res)
}

// requireAffected turns a zero-row Exec into ErrNotFound.
func requireAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
