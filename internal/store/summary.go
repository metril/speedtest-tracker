package store

import (
	"context"
	"database/sql"
	"fmt"
)

// TargetSummary is one target's dashboard row: its latest result plus the
// aggregates over the requested window.
type TargetSummary struct {
	TargetID       int64   `json:"target_id"`
	TargetName     string  `json:"target_name"`
	Engine         string  `json:"engine"`
	Latest         *Result `json:"latest"`
	Count          int     `json:"count"`
	FailCount      int     `json:"fail_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDownloadBps float64 `json:"avg_download_bps"`
	MinDownloadBps float64 `json:"min_download_bps"`
	MaxDownloadBps float64 `json:"max_download_bps"`
	AvgUploadBps   float64 `json:"avg_upload_bps"`
	AvgPingMs      float64 `json:"avg_ping_ms"`
	MaxPingMs      float64 `json:"max_ping_ms"`
}

// SummaryStats is the /stats/summary payload.
type SummaryStats struct {
	From          string          `json:"from"`
	To            string          `json:"to"`
	Targets       []TargetSummary `json:"targets"`
	TotalResults  int             `json:"total_results"`
	TotalFailures int             `json:"total_failures"`
	SuccessRate   float64         `json:"success_rate"`
}

// Summary returns per-target aggregates over [from,to] plus each target's
// latest result (which may predate the window, so a quiet target still
// shows its last known state). Two queries, never one per target.
func (s *Store) Summary(ctx context.Context, from, to string) (*SummaryStats, error) {
	rows, err := s.Read.QueryContext(ctx, `
		SELECT t.id, t.name, t.engine,
		       COUNT(r.id),
		       SUM(CASE WHEN r.status IS NOT NULL AND r.status <> 'ok' THEN 1 ELSE 0 END),
		       AVG(CASE WHEN r.status='ok' THEN r.download_bps END),
		       MIN(CASE WHEN r.status='ok' THEN r.download_bps END),
		       MAX(CASE WHEN r.status='ok' THEN r.download_bps END),
		       AVG(CASE WHEN r.status='ok' THEN r.upload_bps END),
		       AVG(CASE WHEN r.status='ok' THEN r.ping_ms END),
		       MAX(CASE WHEN r.status='ok' THEN r.ping_ms END)
		FROM targets t
		LEFT JOIN results r
		  ON r.target_id = t.id AND r.started_at >= ? AND r.started_at <= ?
		GROUP BY t.id
		ORDER BY t.name, t.id`, from, to)
	if err != nil {
		return nil, fmt.Errorf("summary aggregates: %w", err)
	}
	defer rows.Close()

	out := &SummaryStats{From: from, To: to, Targets: []TargetSummary{}}
	for rows.Next() {
		var (
			ts               TargetSummary
			fails            sql.NullInt64
			avgD, minD, maxD sql.NullFloat64
			avgU, avgP, maxP sql.NullFloat64
		)
		if err := rows.Scan(&ts.TargetID, &ts.TargetName, &ts.Engine, &ts.Count, &fails,
			&avgD, &minD, &maxD, &avgU, &avgP, &maxP); err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}
		ts.FailCount = int(fails.Int64)
		ts.AvgDownloadBps, ts.MinDownloadBps, ts.MaxDownloadBps = avgD.Float64, minD.Float64, maxD.Float64
		ts.AvgUploadBps, ts.AvgPingMs, ts.MaxPingMs = avgU.Float64, avgP.Float64, maxP.Float64
		if ts.Count > 0 {
			ts.SuccessRate = float64(ts.Count-ts.FailCount) / float64(ts.Count)
		}
		out.Targets = append(out.Targets, ts)
		out.TotalResults += ts.Count
		out.TotalFailures += ts.FailCount
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out.TotalResults > 0 {
		out.SuccessRate = float64(out.TotalResults-out.TotalFailures) / float64(out.TotalResults)
	}

	latest, err := s.LatestResults(ctx)
	if err != nil {
		return nil, err
	}
	byTarget := make(map[int64]*Result, len(latest))
	for i := range latest {
		if latest[i].TargetID != nil {
			byTarget[*latest[i].TargetID] = &latest[i]
		}
	}
	for i := range out.Targets {
		out.Targets[i].Latest = byTarget[out.Targets[i].TargetID]
	}
	return out, nil
}
