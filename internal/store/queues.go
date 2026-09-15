package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Queue is a user-managed run lane: targets in the same queue run one at a
// time, and different queues run in parallel.
type Queue struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ErrQueueInUse is returned by DeleteQueue when a target still references
// the queue.
var ErrQueueInUse = errors.New("store: queue in use")

// ErrLastQueue is returned by DeleteQueue when it would remove the only
// remaining queue: every target must always have somewhere to run.
var ErrLastQueue = errors.New("store: cannot delete the last queue")

// queueNameConflict maps SQLite's UNIQUE violation on queues.name onto
// ErrNameConflict so the API can answer 409 instead of 500.
func queueNameConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: queues.name") {
		return ErrNameConflict
	}
	return err
}

func scanQueue(sc interface{ Scan(...any) error }) (Queue, error) {
	var q Queue
	err := sc.Scan(&q.ID, &q.Name, &q.CreatedAt)
	return q, err
}

// ListQueues returns every queue ordered by id, i.e. oldest (and the
// default) first.
func (s *Store) ListQueues(ctx context.Context) ([]Queue, error) {
	rows, err := s.Read.QueryContext(ctx, `SELECT id,name,created_at FROM queues ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer rows.Close()
	out := []Queue{}
	for rows.Next() {
		q, err := scanQueue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetQueue returns the queue with the given id, or ErrNotFound.
func (s *Store) GetQueue(ctx context.Context, id int64) (Queue, error) {
	q, err := scanQueue(s.Read.QueryRowContext(ctx, `SELECT id,name,created_at FROM queues WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Queue{}, ErrNotFound
	}
	if err != nil {
		return Queue{}, fmt.Errorf("get queue %d: %w", id, err)
	}
	return q, nil
}

// GetQueueByName returns the queue with the given (exact) name, or
// ErrNotFound.
func (s *Store) GetQueueByName(ctx context.Context, name string) (Queue, error) {
	q, err := scanQueue(s.Read.QueryRowContext(ctx, `SELECT id,name,created_at FROM queues WHERE name=?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return Queue{}, ErrNotFound
	}
	if err != nil {
		return Queue{}, fmt.Errorf("get queue %q: %w", name, err)
	}
	return q, nil
}

// DefaultQueueID returns the id of the default queue new/legacy targets
// fall back to: the lowest-id queue, i.e. whichever queue was created
// first (seeded as "wan" by migration 0007, but resolved structurally so
// it keeps working even if that queue is later renamed).
func (s *Store) DefaultQueueID(ctx context.Context) (int64, error) {
	var id int64
	err := s.Read.QueryRowContext(ctx, `SELECT id FROM queues ORDER BY id LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("default queue: %w", err)
	}
	return id, nil
}

// CreateQueue inserts a new queue, or returns ErrNameConflict if the name
// is already taken.
func (s *Store) CreateQueue(ctx context.Context, name string) (Queue, error) {
	res, err := s.Write.ExecContext(ctx, `INSERT INTO queues(name) VALUES(?)`, name)
	if err != nil {
		return Queue{}, queueNameConflict(fmt.Errorf("insert queue: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Queue{}, fmt.Errorf("insert queue: %w", err)
	}
	return s.GetQueue(ctx, id)
}

// RenameQueue renames a queue in place, or returns ErrNotFound /
// ErrNameConflict.
func (s *Store) RenameQueue(ctx context.Context, id int64, name string) (Queue, error) {
	res, err := s.Write.ExecContext(ctx, `UPDATE queues SET name=? WHERE id=?`, name, id)
	if err != nil {
		return Queue{}, queueNameConflict(fmt.Errorf("rename queue %d: %w", id, err))
	}
	if err := requireAffected(res); err != nil {
		return Queue{}, err
	}
	return s.GetQueue(ctx, id)
}

// DeleteQueue removes a queue, or returns ErrNotFound, ErrQueueInUse (a
// target still references it) or ErrLastQueue (it is the only queue
// left).
func (s *Store) DeleteQueue(ctx context.Context, id int64) error {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete queue %d: %w", id, err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM queues WHERE id=?`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("check queue %d exists: %w", id, err)
	}

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM queues`).Scan(&total); err != nil {
		return fmt.Errorf("count queues: %w", err)
	}
	if total <= 1 {
		return ErrLastQueue
	}

	var inUse int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM targets WHERE queue_id=?`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("check queue %d in use: %w", id, err)
	}
	if inUse > 0 {
		return ErrQueueInUse
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM queues WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete queue %d: %w", id, err)
	}
	return tx.Commit()
}

// ResolveSnapshotQueueID resolves the queue a stored Target JSON snapshot
// belongs to. Snapshots written since queues were introduced carry
// "queue_id" directly; older ones carry only "lane", a queue name to
// resolve by lookup. Either way, an id that no longer names a live queue
// (e.g. lane's queue was since deleted) falls back to DefaultQueueID.
func (s *Store) ResolveSnapshotQueueID(ctx context.Context, raw json.RawMessage) (int64, error) {
	var probe struct {
		QueueID int64  `json:"queue_id"`
		Lane    string `json:"lane"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 0, fmt.Errorf("parse snapshot: %w", err)
	}
	if probe.QueueID != 0 {
		if _, err := s.GetQueue(ctx, probe.QueueID); err == nil {
			return probe.QueueID, nil
		}
	}
	if probe.Lane != "" {
		if q, err := s.GetQueueByName(ctx, probe.Lane); err == nil {
			return q.ID, nil
		}
	}
	return s.DefaultQueueID(ctx)
}
