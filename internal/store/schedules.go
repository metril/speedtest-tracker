package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Schedule is a named cron schedule over an ordered list of targets.
type Schedule struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Cron      string       `json:"cron"`
	Enabled   bool         `json:"enabled"`
	Timezone  string       `json:"timezone"`
	TargetIDs []int64      `json:"target_ids"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
	LastRun   *ScheduleRun `json:"last_run"`
}

// ScheduleRun is the compact "last run" badge the schedules listing shows.
type ScheduleRun struct {
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
}

const scheduleColumns = `id,name,cron,enabled,timezone,created_at,updated_at`

func scanSchedule(sc interface{ Scan(...any) error }) (*Schedule, error) {
	var s Schedule
	if err := sc.Scan(&s.ID, &s.Name, &s.Cron, &s.Enabled, &s.Timezone,
		&s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.TargetIDs = []int64{}
	return &s, nil
}

// nameConflict maps SQLite's UNIQUE violation on schedules.name onto
// ErrNameConflict so the API can answer 409 instead of 500.
func nameConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: schedules.name") {
		return ErrNameConflict
	}
	return err
}

// replaceScheduleTargets rewrites the ordered target list inside tx.
func replaceScheduleTargets(ctx context.Context, tx *sql.Tx, scheduleID int64, targetIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM schedule_targets WHERE schedule_id=?`, scheduleID); err != nil {
		return fmt.Errorf("clear schedule targets: %w", err)
	}
	for pos, tid := range targetIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schedule_targets(schedule_id,target_id,position) VALUES(?,?,?)`,
			scheduleID, tid, pos); err != nil {
			return fmt.Errorf("insert schedule target %d: %w", tid, err)
		}
	}
	return nil
}

// CreateSchedule inserts the schedule and its ordered targets in one
// transaction and returns the new id.
func (s *Store) CreateSchedule(ctx context.Context, sc *Schedule) (int64, error) {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin schedule tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO schedules(name,cron,enabled,timezone) VALUES(?,?,?,?)`,
		sc.Name, sc.Cron, sc.Enabled, tzOrUTC(sc.Timezone))
	if err != nil {
		return 0, nameConflict(fmt.Errorf("insert schedule: %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceScheduleTargets(ctx, tx, id, sc.TargetIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit schedule: %w", err)
	}
	sc.ID = id
	return id, nil
}

// tzOrUTC normalises an empty timezone to UTC (the column is NOT NULL).
func tzOrUTC(tz string) string {
	if strings.TrimSpace(tz) == "" {
		return "UTC"
	}
	return tz
}

// UpdateSchedule writes every mutable field and replaces the target list in
// one transaction, or returns ErrNotFound.
func (s *Store) UpdateSchedule(ctx context.Context, sc *Schedule) error {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schedule tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE schedules SET name=?,cron=?,enabled=?,timezone=?,
			updated_at=`+nowExpr+`
		WHERE id=?`, sc.Name, sc.Cron, sc.Enabled, tzOrUTC(sc.Timezone), sc.ID)
	if err != nil {
		return nameConflict(fmt.Errorf("update schedule %d: %w", sc.ID, err))
	}
	if err := requireAffected(res); err != nil {
		return err
	}
	if err := replaceScheduleTargets(ctx, tx, sc.ID, sc.TargetIDs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schedule: %w", err)
	}
	return nil
}

// GetSchedule returns the schedule with its ordered targets, or ErrNotFound.
func (s *Store) GetSchedule(ctx context.Context, id int64) (*Schedule, error) {
	row := s.Read.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE id=?`, id)
	sc, err := scanSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get schedule %d: %w", id, err)
	}
	one := []Schedule{*sc}
	if err := s.attachScheduleTargets(ctx, one); err != nil {
		return nil, err
	}
	if err := s.attachLastRuns(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

// ListSchedules returns every schedule ordered by name, targets attached.
func (s *Store) ListSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.Read.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM schedules ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		out = append(out, *sc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachScheduleTargets(ctx, out); err != nil {
		return nil, err
	}
	if err := s.attachLastRuns(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachLastRuns fills LastRun for every schedule in one query. The
// newest run is the one with the greatest started_at (ties broken by id),
// not simply the greatest id: a backfilled or re-executed run can be
// inserted after a run that started later.
func (s *Store) attachLastRuns(ctx context.Context, list []Schedule) error {
	if len(list) == 0 {
		return nil
	}
	args := make([]any, len(list))
	byID := make(map[int64]*Schedule, len(list))
	for i := range list {
		args[i] = list[i].ID
		byID[list[i].ID] = &list[i]
	}
	placeholders := `(?` + strings.Repeat(",?", len(list)-1) + `)`
	rows, err := s.Read.QueryContext(ctx, `
		SELECT schedule_id, status, COALESCE(started_at,'') FROM (
			SELECT schedule_id, status, started_at,
			       ROW_NUMBER() OVER (
			           PARTITION BY schedule_id
			           ORDER BY COALESCE(started_at,'') DESC, id DESC
			       ) AS rn
			FROM runs
			WHERE schedule_id IN `+placeholders+`
		) WHERE rn = 1`, args...)
	if err != nil {
		return fmt.Errorf("load schedule last runs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid int64
		var lr ScheduleRun
		if err := rows.Scan(&sid, &lr.Status, &lr.StartedAt); err != nil {
			return fmt.Errorf("scan schedule last run: %w", err)
		}
		if sc, ok := byID[sid]; ok {
			run := lr
			sc.LastRun = &run
		}
	}
	return rows.Err()
}

// attachScheduleTargets fills TargetIDs for every schedule in one query,
// ordered by position (never one query per row).
func (s *Store) attachScheduleTargets(ctx context.Context, list []Schedule) error {
	if len(list) == 0 {
		return nil
	}
	args := make([]any, len(list))
	byID := make(map[int64]*Schedule, len(list))
	for i := range list {
		args[i] = list[i].ID
		byID[list[i].ID] = &list[i]
	}
	q := `SELECT schedule_id,target_id FROM schedule_targets WHERE schedule_id IN (?` +
		strings.Repeat(",?", len(list)-1) + `) ORDER BY schedule_id, position`
	rows, err := s.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("load schedule targets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid, tid int64
		if err := rows.Scan(&sid, &tid); err != nil {
			return fmt.Errorf("scan schedule target: %w", err)
		}
		if sc, ok := byID[sid]; ok {
			sc.TargetIDs = append(sc.TargetIDs, tid)
		}
	}
	return rows.Err()
}

// DeleteSchedule removes the schedule (cascading schedule_targets), or
// returns ErrNotFound.
func (s *Store) DeleteSchedule(ctx context.Context, id int64) error {
	res, err := s.Write.ExecContext(ctx, `DELETE FROM schedules WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete schedule %d: %w", id, err)
	}
	return requireAffected(res)
}

// InsertSkippedRun records a cron fire that never ran (a previous run still
// in flight, or a full queue) as a terminal "skipped" run row, so the
// outage view can show the gap.
func (s *Store) InsertSkippedRun(ctx context.Context, scheduleID int64, reason string) (int64, error) {
	res, err := s.Write.ExecContext(ctx, `
		INSERT INTO runs(schedule_id,trigger,status,started_at,finished_at,error)
		VALUES(?,'cron','skipped',`+nowExpr+`,`+nowExpr+`,?)`, scheduleID, nullString(reason))
	if err != nil {
		return 0, fmt.Errorf("insert skipped run: %w", err)
	}
	return res.LastInsertId()
}
