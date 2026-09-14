package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TimeFormat is the timestamp layout every text time column uses (it
// matches nowExpr), so Go-built bounds compare correctly against stored
// values.
const TimeFormat = "2006-01-02T15:04:05.000Z"

// HistoryPoint is one downsampled bucket of a target's results.
type HistoryPoint struct {
	BucketStart    string  `json:"bucket_start"`
	Count          int     `json:"count"`
	FailCount      int     `json:"fail_count"`
	AvgDownloadBps float64 `json:"avg_download_bps"`
	MinDownloadBps float64 `json:"min_download_bps"`
	MaxDownloadBps float64 `json:"max_download_bps"`
	AvgUploadBps   float64 `json:"avg_upload_bps"`
	MinUploadBps   float64 `json:"min_upload_bps"`
	MaxUploadBps   float64 `json:"max_upload_bps"`
	AvgPingMs      float64 `json:"avg_ping_ms"`
	MinPingMs      float64 `json:"min_ping_ms"`
	MaxPingMs      float64 `json:"max_ping_ms"`
	AvgJitterMs    float64 `json:"avg_jitter_ms"`
}

// bucketSteps are the allowed bucket widths, smallest first. They are all
// divisors of an hour or multiples of it so bucket starts line up with
// wall-clock boundaries.
var bucketSteps = []int{60, 300, 900, 1800, 3600, 7200, 21600, 43200, 86400}

// maxHistoryPoints caps what a chart request may return.
const maxHistoryPoints = 500

// BucketSecondsFor picks the smallest standard bucket width that keeps a
// span under maxHistoryPoints points.
func BucketSecondsFor(span time.Duration) int {
	secs := int(span.Seconds())
	if secs <= 0 {
		return bucketSteps[0]
	}
	for _, step := range bucketSteps {
		if secs/step <= maxHistoryPoints {
			return step
		}
	}
	return bucketSteps[len(bucketSteps)-1]
}

// epochExpr converts the stored ISO-8601 text timestamp to unix seconds.
// The trailing Z is stripped because every stored value is already UTC and
// older SQLite builds reject the suffix.
const epochExpr = `CAST(strftime('%s', replace(started_at,'Z','')) AS INTEGER)`

// HistoryBuckets returns per-bucket aggregates for one target between from
// and to (inclusive, TimeFormat strings), grouped in SQL so a 30-day chart
// still returns a few hundred rows. Buckets with no results are omitted;
// the caller draws the gap.
func (s *Store) HistoryBuckets(ctx context.Context, targetID int64, from, to string, bucketSeconds int) ([]HistoryPoint, error) {
	if bucketSeconds <= 0 {
		bucketSeconds = bucketSteps[0]
	}
	rows, err := s.Read.QueryContext(ctx, `
		SELECT (`+epochExpr+` / ?) * ? AS bucket,
		       COUNT(*),
		       SUM(CASE WHEN status <> 'ok' THEN 1 ELSE 0 END),
		       AVG(NULLIF(download_bps,0)), MIN(NULLIF(download_bps,0)), MAX(download_bps),
		       AVG(NULLIF(upload_bps,0)),   MIN(NULLIF(upload_bps,0)),   MAX(upload_bps),
		       AVG(NULLIF(ping_ms,0)),      MIN(NULLIF(ping_ms,0)),      MAX(ping_ms),
		       AVG(NULLIF(jitter_ms,0))
		FROM results
		WHERE target_id=? AND started_at>=? AND started_at<=?
		GROUP BY bucket
		ORDER BY bucket`,
		bucketSeconds, bucketSeconds, targetID, from, to)
	if err != nil {
		return nil, fmt.Errorf("history buckets: %w", err)
	}
	defer rows.Close()

	out := []HistoryPoint{}
	for rows.Next() {
		var (
			bucket                 int64
			p                      HistoryPoint
			avgD, minD, maxD       sql.NullFloat64
			avgU, minU, maxU       sql.NullFloat64
			avgP, minP, maxP, avgJ sql.NullFloat64
		)
		if err := rows.Scan(&bucket, &p.Count, &p.FailCount,
			&avgD, &minD, &maxD, &avgU, &minU, &maxU, &avgP, &minP, &maxP, &avgJ); err != nil {
			return nil, fmt.Errorf("scan history bucket: %w", err)
		}
		p.BucketStart = time.Unix(bucket, 0).UTC().Format(TimeFormat)
		p.AvgDownloadBps, p.MinDownloadBps, p.MaxDownloadBps = avgD.Float64, minD.Float64, maxD.Float64
		p.AvgUploadBps, p.MinUploadBps, p.MaxUploadBps = avgU.Float64, minU.Float64, maxU.Float64
		p.AvgPingMs, p.MinPingMs, p.MaxPingMs = avgP.Float64, minP.Float64, maxP.Float64
		p.AvgJitterMs = avgJ.Float64
		out = append(out, p)
	}
	return out, rows.Err()
}
