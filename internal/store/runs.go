package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Run is one execution of a set of targets.
type Run struct {
	ID         int64   `json:"id"`
	ScheduleID *int64  `json:"schedule_id"`
	Trigger    string  `json:"trigger"`
	Status     string  `json:"status"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Error      *string `json:"error"`
}

const runColumns = `id,schedule_id,trigger,status,started_at,finished_at,error`

// nowExpr is the SQLite expression producing the timestamp format used by
// every table in this schema.
const nowExpr = `strftime('%Y-%m-%dT%H:%M:%fZ','now')`

func scanRun(sc interface{ Scan(...any) error }) (*Run, error) {
	var r Run
	if err := sc.Scan(&r.ID, &r.ScheduleID, &r.Trigger, &r.Status,
		&r.StartedAt, &r.FinishedAt, &r.Error); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateRun inserts a run in the queued state and returns its id.
func (s *Store) CreateRun(ctx context.Context, trigger string, scheduleID *int64) (int64, error) {
	res, err := s.Write.ExecContext(ctx,
		`INSERT INTO runs(schedule_id,trigger,status) VALUES(?,?,'queued')`, scheduleID, trigger)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	return res.LastInsertId()
}

// terminalRunStatuses are the statuses that stamp finished_at.
var terminalRunStatuses = map[string]bool{
	"done": true, "failed": true, "canceled": true, "skipped": true,
}

// SetRunStatus moves the run to status. Entering "running" stamps
// started_at; a terminal status stamps finished_at and stores errMsg
// (empty errMsg is stored as NULL). A run already in a terminal status
// (done|failed|canceled|skipped) can never be moved to another status: that
// transition is rejected with ErrInvalidTransition instead of silently
// overwriting the run's real outcome.
func (s *Store) SetRunStatus(ctx context.Context, id int64, status, errMsg string) error {
	var errVal any
	if errMsg != "" {
		errVal = errMsg
	}
	q := `UPDATE runs SET status=?, error=COALESCE(?,error)`
	switch {
	case status == "running":
		q += `, started_at=COALESCE(started_at,` + nowExpr + `)`
	case terminalRunStatuses[status]:
		q += `, finished_at=` + nowExpr
	}
	q += ` WHERE id=? AND status NOT IN ('done','failed','canceled','skipped')`
	res, err := s.Write.ExecContext(ctx, q, status, errVal, id)
	if err != nil {
		return fmt.Errorf("set run %d status: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n > 0 {
		return nil
	}
	// No row updated: either the run doesn't exist, or it exists but is
	// already terminal and the WHERE clause blocked the transition.
	cur, err := s.GetRun(ctx, id)
	if err != nil {
		return err
	}
	if terminalRunStatuses[cur.Status] {
		return ErrInvalidTransition
	}
	return fmt.Errorf("set run %d status: no rows updated (current status %q)", id, cur.Status)
}

// GetRun returns the run, or ErrNotFound.
func (s *Store) GetRun(ctx context.Context, id int64) (*Run, error) {
	row := s.Read.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runs WHERE id=?`, id)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get run %d: %w", id, err)
	}
	return r, nil
}

// ListRuns returns up to limit runs newest-first. cursor is the id
// returned by the previous page (0 for the first page); the returned
// cursor is 0 when the listing is exhausted. Keyset, never OFFSET.
func (s *Store) ListRuns(ctx context.Context, limit int, cursor int64) ([]Run, int64, error) {
	limit = clampLimit(limit)
	q := `SELECT ` + runColumns + ` FROM runs`
	args := []any{}
	if cursor > 0 {
		q += ` WHERE id < ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan run: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// clampLimit bounds a caller-supplied page size.
func clampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

// QueuedRunForSchedule reports an already-queued run for the schedule, so
// the runner can skip enqueuing a duplicate.
func (s *Store) QueuedRunForSchedule(ctx context.Context, scheduleID int64) (int64, bool, error) {
	var id int64
	err := s.Read.QueryRowContext(ctx,
		`SELECT id FROM runs WHERE schedule_id=? AND status IN ('queued','running') ORDER BY id DESC LIMIT 1`,
		scheduleID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("queued run for schedule %d: %w", scheduleID, err)
	}
	return id, true, nil
}
