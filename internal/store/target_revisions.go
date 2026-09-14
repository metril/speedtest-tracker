package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// maxTargetRevisions is how many revisions insertRevision keeps per
// target; older rows are pruned after each insert.
const maxTargetRevisions = 50

// TargetRevision is one snapshot in a target's change history.
type TargetRevision struct {
	ID        int64           `json:"id"`
	TargetID  int64           `json:"target_id"`
	Version   int             `json:"version"`
	Action    string          `json:"action"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt string          `json:"created_at"`
}

// DeletedTarget summarises a target's last-known state for the
// "recently deleted" list.
type DeletedTarget struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Lane      string `json:"lane"`
	DeletedAt string `json:"deleted_at"`
	Version   int    `json:"version"`
}

func scanTargetRevision(sc interface{ Scan(...any) error }) (*TargetRevision, error) {
	var rev TargetRevision
	var snapshot string
	if err := sc.Scan(&rev.ID, &rev.TargetID, &rev.Version, &rev.Action, &snapshot, &rev.CreatedAt); err != nil {
		return nil, err
	}
	rev.Snapshot = json.RawMessage(snapshot)
	return &rev, nil
}

// insertRevision writes the next version for targetID within tx: version
// is max(existing)+1, and any version <= newVersion-maxTargetRevisions is
// pruned so at most maxTargetRevisions rows survive per target.
func insertRevision(ctx context.Context, tx *sql.Tx, targetID int64, action string, snapshot *Target) error {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	var maxVersion int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version),0) FROM target_revisions WHERE target_id=?`, targetID,
	).Scan(&maxVersion); err != nil {
		return fmt.Errorf("read max version: %w", err)
	}
	newVersion := maxVersion + 1

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO target_revisions(target_id,version,action,snapshot) VALUES(?,?,?,?)`,
		targetID, newVersion, action, string(body),
	); err != nil {
		return fmt.Errorf("insert revision: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM target_revisions WHERE target_id=? AND version<=?`,
		targetID, newVersion-maxTargetRevisions,
	); err != nil {
		return fmt.Errorf("prune revisions: %w", err)
	}
	return nil
}

// ListTargetRevisions returns every revision for id, newest first.
func (s *Store) ListTargetRevisions(ctx context.Context, id int64) ([]TargetRevision, error) {
	rows, err := s.Read.QueryContext(ctx,
		`SELECT id,target_id,version,action,snapshot,created_at
		 FROM target_revisions WHERE target_id=? ORDER BY version DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("list target revisions %d: %w", id, err)
	}
	defer rows.Close()
	out := []TargetRevision{}
	for rows.Next() {
		rev, err := scanTargetRevision(rows)
		if err != nil {
			return nil, fmt.Errorf("scan target revision: %w", err)
		}
		out = append(out, *rev)
	}
	return out, rows.Err()
}

// GetTargetRevision returns one target's revision at version, or
// ErrNotFound.
func (s *Store) GetTargetRevision(ctx context.Context, id int64, version int) (*TargetRevision, error) {
	row := s.Read.QueryRowContext(ctx,
		`SELECT id,target_id,version,action,snapshot,created_at
		 FROM target_revisions WHERE target_id=? AND version=?`, id, version)
	rev, err := scanTargetRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get target revision %d/%d: %w", id, version, err)
	}
	return rev, nil
}

// ListDeletedTargets lists targets whose most recent revision is a
// "delete" and that have no live row — i.e. deleted and not since
// restored.
func (s *Store) ListDeletedTargets(ctx context.Context) ([]DeletedTarget, error) {
	rows, err := s.Read.QueryContext(ctx, `
		SELECT tr.target_id, tr.version, tr.snapshot, tr.created_at
		FROM target_revisions tr
		JOIN (
			SELECT target_id, MAX(version) AS maxv
			FROM target_revisions
			GROUP BY target_id
		) latest ON latest.target_id = tr.target_id AND latest.maxv = tr.version
		WHERE tr.action = 'delete'
		  AND NOT EXISTS (SELECT 1 FROM targets t WHERE t.id = tr.target_id)
		ORDER BY tr.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list deleted targets: %w", err)
	}
	defer rows.Close()

	out := []DeletedTarget{}
	for rows.Next() {
		var targetID int64
		var version int
		var snapshot, createdAt string
		if err := rows.Scan(&targetID, &version, &snapshot, &createdAt); err != nil {
			return nil, fmt.Errorf("scan deleted target: %w", err)
		}
		var t Target
		if err := json.Unmarshal([]byte(snapshot), &t); err != nil {
			return nil, fmt.Errorf("unmarshal deleted target %d snapshot: %w", targetID, err)
		}
		out = append(out, DeletedTarget{
			ID: targetID, Name: t.Name, Engine: t.Engine, Lane: t.Lane,
			DeletedAt: createdAt, Version: version,
		})
	}
	return out, rows.Err()
}

// LatestDeletedSnapshot returns the full snapshot backing id's entry in
// ListDeletedTargets, or ErrNotFound if id is not currently deleted.
func (s *Store) LatestDeletedSnapshot(ctx context.Context, id int64) (*Target, error) {
	row := s.Read.QueryRowContext(ctx, `
		SELECT tr.snapshot
		FROM target_revisions tr
		JOIN (
			SELECT target_id, MAX(version) AS maxv
			FROM target_revisions
			WHERE target_id=?
			GROUP BY target_id
		) latest ON latest.target_id = tr.target_id AND latest.maxv = tr.version
		WHERE tr.action = 'delete'
		  AND NOT EXISTS (SELECT 1 FROM targets t WHERE t.id = tr.target_id)`, id)
	var snapshot string
	if err := row.Scan(&snapshot); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("latest deleted snapshot %d: %w", id, err)
	}
	var t Target
	if err := json.Unmarshal([]byte(snapshot), &t); err != nil {
		return nil, fmt.Errorf("unmarshal deleted target %d snapshot: %w", id, err)
	}
	return &t, nil
}

// RestoreTarget re-inserts t with its original id (an explicit id
// INSERT), writes a "restore" revision, and returns the restored row.
// It fails with ErrIDConflict if a live row already occupies t.ID.
func (s *Store) RestoreTarget(ctx context.Context, t *Target) (*Target, error) {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin restore target %d: %w", t.ID, err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM targets WHERE id=?`, t.ID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check live target %d: %w", t.ID, err)
	}
	if exists > 0 {
		return nil, ErrIDConflict
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO targets(id,name,engine,enabled,lane,options,thresholds) VALUES(?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.Engine, t.Enabled, t.Lane, rawOrEmpty(t.Options), rawOrEmpty(t.Thresholds),
	); err != nil {
		return nil, fmt.Errorf("insert restored target %d: %w", t.ID, err)
	}

	restored, err := scanTarget(tx.QueryRowContext(ctx, `SELECT `+targetColumns+` FROM targets WHERE id=?`, t.ID))
	if err != nil {
		return nil, fmt.Errorf("read restored target %d: %w", t.ID, err)
	}
	if err := insertRevision(ctx, tx, t.ID, "restore", restored); err != nil {
		return nil, fmt.Errorf("insert restore revision for target %d: %w", t.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit restore target %d: %w", t.ID, err)
	}
	return restored, nil
}
