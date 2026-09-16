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

	// SLACompliance is the fraction (0..1) of results in the window
	// (status 'ok', 'degraded' or 'failed') that count toward this
	// target's effective SLA plan (its own Thresholds override, falling
	// back to the SLAPlan passed to Summary): a 'failed' row always
	// counts as a miss, while an 'ok'/'degraded' row is judged on
	// whether its download and upload speeds both met the plan, reduced
	// by the effective tolerance percent. nil when neither the target
	// nor the general plan set a download or upload speed, or when there
	// were no results in the window that counted toward the plan.
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
	// target's own SLA-counted result count (see TargetSummary.SLACompliance),
	// across every target with a resolved SLA plan. nil when no target has
	// one.
	SLACompliance *float64 `json:"sla_compliance"`
}

// SLAPlan is the general (fallback) SLA plan, used to resolve a target's
// effective plan when its own Thresholds don't set the matching field.
// DownloadMbps/UploadMbps are in Mbps and either may be nil. TolerancePct
// is a percent (0..99) shrinking the effective threshold
// (plan * (1 - tol/100)); unlike the speed fields, nil and 0 are distinct
// (nil means "no general tolerance", equivalent to 0).
type SLAPlan struct {
	DownloadMbps *float64
	UploadMbps   *float64
	TolerancePct *float64
}

// targetSLAOverride mirrors the two SLA fields of settings.Thresholds
// (sla_download_mbps/sla_upload_mbps). Duplicated here, rather than
// imported, because internal/settings imports this package.
type targetSLAOverride struct {
	SLADownloadMbps *float64 `json:"sla_download_mbps"`
	SLAUploadMbps   *float64 `json:"sla_upload_mbps"`
	SLATolerancePct *float64 `json:"sla_tolerance_pct"`
}

// clampTolerancePct clamps a tolerance percent to the valid 0..99 range.
func clampTolerancePct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 99 {
		return 99
	}
	return v
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
// unset (see normalizeMbps). downloadMbps/uploadMbps are nil when neither
// the target nor the general plan set that field. tolerancePct is resolved
// separately (it is a real value at 0, so it is never routed through
// normalizeMbps): the target's own override wins if present (non-nil,
// including an explicit 0), else the general tolerance, else 0 — always
// clamped to 0..99.
func resolvePlan(thresholdsJSON string, general SLAPlan) (downloadMbps, uploadMbps *float64, tolerancePct float64) {
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
	switch {
	case t.SLATolerancePct != nil:
		tolerancePct = *t.SLATolerancePct
	case general.TolerancePct != nil:
		tolerancePct = *general.TolerancePct
	default:
		tolerancePct = 0
	}
	return downloadMbps, uploadMbps, clampTolerancePct(tolerancePct)
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
	// planDL/planUL/planTol per target, keyed by target id, resolved once
	// here so the SLA pass below (over individual results) doesn't need to
	// re-parse thresholds JSON per row.
	planDL := map[int64]*float64{}
	planUL := map[int64]*float64{}
	planTol := map[int64]float64{}
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
		planDL[ts.TargetID], planUL[ts.TargetID], planTol[ts.TargetID] = resolvePlan(thresholdsJSON, sla)
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

	// SLA compliance pass: one query over every result in the window
	// (across all targets, any status), tallied per target against its
	// already-resolved plan.
	slaDenom := map[int64]int{}
	slaCompliant := map[int64]int{}
	slaRows, err := s.Read.QueryContext(ctx, `
		SELECT target_id, status, COALESCE(download_bps,0), COALESCE(upload_bps,0) FROM results
		WHERE target_id IS NOT NULL AND started_at >= ? AND started_at <= ?`, from, to)
	if err != nil {
		return nil, fmt.Errorf("summary sla results: %w", err)
	}
	for slaRows.Next() {
		var targetID int64
		var status string
		var downloadBps, uploadBps float64
		if err := slaRows.Scan(&targetID, &status, &downloadBps, &uploadBps); err != nil {
			slaRows.Close()
			return nil, fmt.Errorf("scan summary sla row: %w", err)
		}
		dl, ul := planDL[targetID], planUL[targetID]
		if dl == nil && ul == nil {
			continue // no plan resolves for this target: not counted
		}
		// A failed row always counts as a miss: it never had a chance to
		// meet the plan, so it goes straight into the denominator without
		// being judged on its (likely zero/partial) speeds. This check
		// must stay after the no-plan guard above, so a plan-less target's
		// failed rows still don't count (SLACompliance stays nil for it).
		if status == "failed" {
			slaDenom[targetID]++
			continue
		}
		// A direction whose measured value is exactly 0 means the engine
		// never measured it (e.g. a reverse-only or forward-only iperf3
		// run leaves the other direction's *_bps at 0 — see
		// internal/engine/iperf3/parse.go's parseSummary), not that it
		// measured a genuine 0bps: skip that direction's criterion rather
		// than count it as a miss. If neither applicable direction was
		// actually measured, the result says nothing about plan
		// compliance and is excluded from the denominator entirely. This
		// applies to 'ok' and 'degraded' rows alike — both are judged on
		// their measurements the same way.
		checkDL := dl != nil && downloadBps != 0
		checkUL := ul != nil && uploadBps != 0
		if !checkDL && !checkUL {
			continue
		}
		slaDenom[targetID]++
		tol := planTol[targetID]
		thresholdDL, thresholdUL := 0.0, 0.0
		if dl != nil {
			thresholdDL = *dl * 1e6 * (1 - tol/100)
		}
		if ul != nil {
			thresholdUL = *ul * 1e6 * (1 - tol/100)
		}
		if (!checkDL || downloadBps >= thresholdDL) && (!checkUL || uploadBps >= thresholdUL) {
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
