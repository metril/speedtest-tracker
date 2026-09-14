package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

	// SLACompliance is the fraction (0..1) of successful (status='ok')
	// results in the window whose download and upload speeds both met
	// this target's effective SLA plan (its own Thresholds override,
	// falling back to the SLAPlan passed to Summary). nil when neither
	// the target nor the general plan set a download or upload speed, or
	// when there were no successful results in the window to judge.
	SLACompliance *float64 `json:"sla_compliance"`
}

// SummaryStats is the /stats/summary payload.
type SummaryStats struct {
	From          string          `json:"from"`
	To            string          `json:"to"`
	Targets       []TargetSummary `json:"targets"`
	TotalResults  int             `json:"total_results"`
	TotalFailures int             `json:"total_failures"`
	SuccessRate   float64         `json:"success_rate"`

	// SLACompliance is the overall fraction (0..1), weighted by each
	// target's own successful-result count, across every target with a
	// resolved SLA plan. nil when no target has one.
	SLACompliance *float64 `json:"sla_compliance"`
}

// SLAPlan is the general (fallback) SLA plan speeds, in Mbps, used to
// resolve a target's effective plan when its own Thresholds don't set
// sla_download_mbps/sla_upload_mbps. Either field may be nil.
type SLAPlan struct {
	DownloadMbps *float64
	UploadMbps   *float64
}

// targetSLAOverride mirrors the two SLA fields of settings.Thresholds
// (sla_download_mbps/sla_upload_mbps). Duplicated here, rather than
// imported, because internal/settings imports this package.
type targetSLAOverride struct {
	SLADownloadMbps *float64 `json:"sla_download_mbps"`
	SLAUploadMbps   *float64 `json:"sla_upload_mbps"`
}

// normalizeMbps treats a non-positive plan speed as unset: <PUT>ting a plan
// field to 0 (or a negative value slipping through) is the documented way
// to disable/clear it, since the settings API can't distinguish an
// omitted field from an explicit JSON null on a plain pointer field (see
// internal/api/settings.go's settingsBody.General SLA fields).
func normalizeMbps(v *float64) *float64 {
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

// resolvePlan resolves a target's effective SLA plan: its own override
// (parsed from its thresholds JSON) takes precedence per-field over the
// general plan, and a non-positive value at either level is treated as
// unset (see normalizeMbps). Both returned pointers are nil when neither
// the target nor the general plan set that field.
func resolvePlan(thresholdsJSON string, general SLAPlan) (downloadMbps, uploadMbps *float64) {
	var t targetSLAOverride
	if thresholdsJSON != "" {
		_ = json.Unmarshal([]byte(thresholdsJSON), &t) // malformed thresholds: treat as no override
	}
	downloadMbps = normalizeMbps(t.SLADownloadMbps)
	if downloadMbps == nil {
		downloadMbps = normalizeMbps(general.DownloadMbps)
	}
	uploadMbps = normalizeMbps(t.SLAUploadMbps)
	if uploadMbps == nil {
		uploadMbps = normalizeMbps(general.UploadMbps)
	}
	return downloadMbps, uploadMbps
}

// Summary returns per-target aggregates over [from,to] plus each target's
// latest result (which may predate the window, so a quiet target still
// shows its last known state). sla is the general SLA plan, resolved per
// target against its own Thresholds override (see resolvePlan). Three
// queries total, never one per target.
func (s *Store) Summary(ctx context.Context, from, to string, sla SLAPlan) (*SummaryStats, error) {
	rows, err := s.Read.QueryContext(ctx, `
		SELECT t.id, t.name, t.engine, t.thresholds,
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
	// planDL/planUL per target, keyed by target id, resolved once here so
	// the SLA pass below (over individual results) doesn't need to
	// re-parse thresholds JSON per row.
	planDL := map[int64]*float64{}
	planUL := map[int64]*float64{}
	for rows.Next() {
		var (
			ts               TargetSummary
			thresholdsJSON   string
			fails            sql.NullInt64
			avgD, minD, maxD sql.NullFloat64
			avgU, avgP, maxP sql.NullFloat64
		)
		if err := rows.Scan(&ts.TargetID, &ts.TargetName, &ts.Engine, &thresholdsJSON, &ts.Count, &fails,
			&avgD, &minD, &maxD, &avgU, &avgP, &maxP); err != nil {
			return nil, fmt.Errorf("scan summary row: %w", err)
		}
		ts.FailCount = int(fails.Int64)
		ts.AvgDownloadBps, ts.MinDownloadBps, ts.MaxDownloadBps = avgD.Float64, minD.Float64, maxD.Float64
		ts.AvgUploadBps, ts.AvgPingMs, ts.MaxPingMs = avgU.Float64, avgP.Float64, maxP.Float64
		if ts.Count > 0 {
			ts.SuccessRate = float64(ts.Count-ts.FailCount) / float64(ts.Count)
		}
		planDL[ts.TargetID], planUL[ts.TargetID] = resolvePlan(thresholdsJSON, sla)
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

	// SLA compliance pass: one query over every successful result in the
	// window (across all targets), tallied per target against its
	// already-resolved plan.
	slaDenom := map[int64]int{}
	slaCompliant := map[int64]int{}
	slaRows, err := s.Read.QueryContext(ctx, `
		SELECT target_id, COALESCE(download_bps,0), COALESCE(upload_bps,0) FROM results
		WHERE status='ok' AND target_id IS NOT NULL AND started_at >= ? AND started_at <= ?`, from, to)
	if err != nil {
		return nil, fmt.Errorf("summary sla results: %w", err)
	}
	for slaRows.Next() {
		var targetID int64
		var downloadBps, uploadBps float64
		if err := slaRows.Scan(&targetID, &downloadBps, &uploadBps); err != nil {
			slaRows.Close()
			return nil, fmt.Errorf("scan summary sla row: %w", err)
		}
		dl, ul := planDL[targetID], planUL[targetID]
		if dl == nil && ul == nil {
			continue // no plan resolves for this target: not counted
		}
		// A direction whose measured value is exactly 0 means the engine
		// never measured it (e.g. a reverse-only or forward-only iperf3
		// run leaves the other direction's *_bps at 0 — see
		// internal/engine/iperf3/parse.go's parseSummary), not that it
		// measured a genuine 0bps: skip that direction's criterion rather
		// than count it as a miss. If neither applicable direction was
		// actually measured, the result says nothing about plan
		// compliance and is excluded from the denominator entirely.
		checkDL := dl != nil && downloadBps != 0
		checkUL := ul != nil && uploadBps != 0
		if !checkDL && !checkUL {
			continue
		}
		slaDenom[targetID]++
		if (!checkDL || downloadBps >= *dl*1e6) && (!checkUL || uploadBps >= *ul*1e6) {
			slaCompliant[targetID]++
		}
	}
	if err := slaRows.Err(); err != nil {
		slaRows.Close()
		return nil, fmt.Errorf("summary sla results: %w", err)
	}
	slaRows.Close()

	var totalDenom, totalCompliant int
	for i := range out.Targets {
		id := out.Targets[i].TargetID
		if denom := slaDenom[id]; denom > 0 {
			frac := float64(slaCompliant[id]) / float64(denom)
			out.Targets[i].SLACompliance = &frac
			totalDenom += denom
			totalCompliant += slaCompliant[id]
		}
	}
	if totalDenom > 0 {
		frac := float64(totalCompliant) / float64(totalDenom)
		out.SLACompliance = &frac
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
