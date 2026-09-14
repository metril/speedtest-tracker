package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Incident is a contiguous stretch of trouble: consecutive non-ok results
// for one target (Kind "result"), or a single cron fire that never ran
// (Kind "skipped", named after the schedule).
type Incident struct {
	TargetID   *int64 `json:"target_id"`
	TargetName string `json:"target_name"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	Count      int    `json:"count"`
	Error      string `json:"error,omitempty"`
}

// Outages returns the trouble timeline between from and to. Consecutive
// failed/degraded results for the same target that are no more than
// gapSeconds apart (the caller passes 2x the expected test interval)
// collapse into one incident; a longer quiet stretch starts a new one.
func (s *Store) Outages(ctx context.Context, from, to string, gapSeconds int) ([]Incident, error) {
	if gapSeconds <= 0 {
		gapSeconds = 1800
	}
	rows, err := s.Read.QueryContext(ctx, `
		SELECT target_id, target_name, status, started_at, COALESCE(error,'')
		FROM results
		WHERE status <> 'ok' AND started_at >= ? AND started_at <= ?
		ORDER BY target_id, started_at`, from, to)
	if err != nil {
		return nil, fmt.Errorf("outage results: %w", err)
	}
	defer rows.Close()

	out := []Incident{}
	var cur *Incident
	var curEnd time.Time
	gap := time.Duration(gapSeconds) * time.Second

	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for rows.Next() {
		var (
			tid               sql.NullInt64
			name, status      string
			startedAt, errMsg string
		)
		if err := rows.Scan(&tid, &name, &status, &startedAt, &errMsg); err != nil {
			return nil, fmt.Errorf("scan outage row: %w", err)
		}
		at, err := time.Parse(TimeFormat, startedAt)
		if err != nil {
			return nil, fmt.Errorf("parse outage time %q: %w", startedAt, err)
		}
		sameTarget := cur != nil && ((cur.TargetID == nil && !tid.Valid) ||
			(cur.TargetID != nil && tid.Valid && *cur.TargetID == tid.Int64))
		if sameTarget && at.Sub(curEnd) <= gap {
			cur.EndedAt = startedAt
			cur.Count++
			if cur.Status == "degraded" && status == "failed" {
				cur.Status = "failed" // the worse status wins
			}
			if cur.Error == "" {
				cur.Error = errMsg
			}
			curEnd = at
			continue
		}
		flush()
		inc := Incident{TargetName: name, Kind: "result", Status: status,
			StartedAt: startedAt, EndedAt: startedAt, Count: 1, Error: errMsg}
		if tid.Valid {
			id := tid.Int64
			inc.TargetID = &id
		}
		cur, curEnd = &inc, at
	}
	flush()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	skipped, err := s.Read.QueryContext(ctx, `
		SELECT COALESCE(s.name,'(deleted schedule)'), COALESCE(r.started_at,''), COALESCE(r.error,'skipped')
		FROM runs r
		LEFT JOIN schedules s ON s.id = r.schedule_id
		WHERE r.status='skipped' AND r.started_at >= ? AND r.started_at <= ?
		ORDER BY r.started_at`, from, to)
	if err != nil {
		return nil, fmt.Errorf("outage skipped runs: %w", err)
	}
	defer skipped.Close()
	for skipped.Next() {
		var name, startedAt, reason string
		if err := skipped.Scan(&name, &startedAt, &reason); err != nil {
			return nil, fmt.Errorf("scan skipped run: %w", err)
		}
		out = append(out, Incident{TargetName: name, Kind: "skipped", Status: "skipped",
			StartedAt: startedAt, EndedAt: startedAt, Count: 1, Error: reason})
	}
	if err := skipped.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
