package api

import (
	"net/http"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// namedRanges are the chart presets the UI offers.
var namedRanges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// defaultOutageGapSeconds is twice a 15-minute schedule: failures closer
// together than this belong to the same incident.
const defaultOutageGapSeconds = 1800

// rangeWindow resolves ?range=24h|7d|30d, or an explicit ?from&to pair, to
// a concrete window. It answers 400 itself and reports whether the window
// is usable.
func rangeWindow(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	from, ok := timeQuery(w, r, "from")
	if !ok {
		return "", "", false
	}
	to, ok := timeQuery(w, r, "to")
	if !ok {
		return "", "", false
	}
	if from != "" && to != "" {
		if from > to {
			errBadRequest(w, "from must be before to")
			return "", "", false
		}
		return from, to, true
	}
	name := r.URL.Query().Get("range")
	if name == "" {
		name = "24h"
	}
	span, known := namedRanges[name]
	if !known {
		errBadRequest(w, "range must be one of 24h, 7d, 30d, or an explicit from/to pair")
		return "", "", false
	}
	now := time.Now().UTC().Truncate(summaryTTL)
	return now.Add(-span).Format(dbTimeFormat), now.Format(dbTimeFormat), true
}

// windowSpan is the duration between two dbTimeFormat bounds.
func windowSpan(from, to string) time.Duration {
	f, err1 := time.Parse(dbTimeFormat, from)
	t, err2 := time.Parse(dbTimeFormat, to)
	if err1 != nil || err2 != nil || !t.After(f) {
		return time.Hour
	}
	return t.Sub(f)
}

// targetHistory answers GET /targets/{id}/history: buckets computed in
// SQL, so the response is bounded no matter how long the window is.
func (d Deps) targetHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := d.Store.GetTarget(r.Context(), id); storeError(w, d.Logger, "target", err) {
		return
	}
	from, to, ok := rangeWindow(w, r)
	if !ok {
		return
	}
	bucket := store.BucketSecondsFor(windowSpan(from, to))
	points, err := d.Store.HistoryBuckets(r.Context(), id, from, to, bucket)
	if err != nil {
		internalError(w, d.Logger, "history failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"target_id": id, "from": from, "to": to,
		"bucket_seconds": bucket, "points": points,
	})
}

// outages answers GET /outages?from&to&gap_seconds.
func (d Deps) outages(w http.ResponseWriter, r *http.Request) {
	from, to, ok := rangeWindow(w, r)
	if !ok {
		return
	}
	gap, ok := intQuery(w, r, "gap_seconds", defaultOutageGapSeconds)
	if !ok {
		return
	}
	if gap <= 0 {
		errBadRequest(w, "gap_seconds must be positive")
		return
	}
	incidents, err := d.Store.Outages(r.Context(), from, to, gap)
	if err != nil {
		internalError(w, d.Logger, "outages failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "incidents": incidents})
}
