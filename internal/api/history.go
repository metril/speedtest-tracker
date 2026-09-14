package api

import (
	"net/http"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// shiftWindow shifts [from,to] (dbTimeFormat bounds) back by its own span
// when offset is 1, giving the caller the immediately preceding period of
// equal length for a previous-period comparison; offset 0 returns from,to
// unchanged. Store queries are inclusive on both ends ([from,to]), so the
// shifted window's "to" is nudged back one millisecond (the stored
// timestamps' precision) to stay adjacent rather than overlapping the
// current window. ok is false (with from/to both "") only when from can't
// be parsed — callers should treat that as an internal invariant
// violation, since rangeWindow always returns a parseable from.
func shiftWindow(from, to string, offset int) (string, string, bool) {
	if offset == 0 {
		return from, to, true
	}
	span := windowSpan(from, to)
	f, err := time.Parse(dbTimeFormat, from)
	if err != nil {
		return "", "", false
	}
	to = f.Add(-time.Millisecond).Format(dbTimeFormat)
	from = f.Add(-span).Format(dbTimeFormat)
	return from, to, true
}

// namedRanges are the chart presets the UI offers.
var namedRanges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// defaultOutageGapSeconds is twice a 15-minute schedule: failures closer
// together than this belong to the same incident.
const defaultOutageGapSeconds = 1800

// Default window spans used when a request gives neither range nor from/to.
const (
	defaultHistorySpan = 24 * time.Hour
	defaultOutagesSpan = 7 * 24 * time.Hour
)

// maxExplicitSpan bounds an explicit ?from&to pair for /history and
// /outages: unlike the named ranges (capped at 30d), a caller-chosen span
// could otherwise ask for an unbounded table scan.
const maxExplicitSpan = 90 * 24 * time.Hour

// windowGranularity is the wall-clock resolution rangeWindow truncates
// "now" to before applying a named range, so two requests in the same
// window share one summary-cache key. It happens to equal summaryTTL: a
// window can't usefully be finer than the cache's own reuse period, but
// the two constants are named separately so that coupling is explicit
// rather than implied by reusing summaryTTL's name here.
const windowGranularity = summaryTTL

// rangeWindow resolves ?range=24h|7d|30d, or an explicit ?from&to pair, to
// a concrete window. defaultSpan is used when neither range nor from/to is
// given, so callers (history vs. outages) can pick their own default. It
// answers 400 itself and reports whether the window is usable.
func rangeWindow(w http.ResponseWriter, r *http.Request, defaultSpan time.Duration) (string, string, bool) {
	from, ok := timeQuery(w, r, "from")
	if !ok {
		return "", "", false
	}
	to, ok := timeQuery(w, r, "to")
	if !ok {
		return "", "", false
	}
	if (from == "") != (to == "") {
		errBadRequest(w, "from and to must be given together")
		return "", "", false
	}
	if from != "" && to != "" {
		if from > to {
			errBadRequest(w, "from must be before to")
			return "", "", false
		}
		if f, err1 := time.Parse(dbTimeFormat, from); err1 == nil {
			if t, err2 := time.Parse(dbTimeFormat, to); err2 == nil && t.Sub(f) > maxExplicitSpan {
				errBadRequest(w, "range too large")
				return "", "", false
			}
		}
		return from, to, true
	}
	name := r.URL.Query().Get("range")
	now := time.Now().UTC().Truncate(windowGranularity)
	if name == "" {
		return now.Add(-defaultSpan).Format(dbTimeFormat), now.Format(dbTimeFormat), true
	}
	span, known := namedRanges[name]
	if !known {
		errBadRequest(w, "range must be one of 24h, 7d, 30d, or an explicit from/to pair")
		return "", "", false
	}
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

// targetHistory answers GET /targets/{id}/history?range=&offset=: buckets
// computed in SQL, so the response is bounded no matter how long the
// window is. offset=1 shifts the resolved window back by its own span
// (see shiftWindow), giving the immediately preceding period of equal
// length — e.g. for a "compare with previous period" chart overlay.
func (d Deps) targetHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := d.Store.GetTarget(r.Context(), id); storeError(w, d.Logger, "target", err) {
		return
	}
	offset, ok := intQuery(w, r, "offset", 0)
	if !ok {
		return
	}
	if offset < 0 || offset > 1 {
		errBadRequest(w, "offset must be 0 or 1")
		return
	}
	from, to, ok := rangeWindow(w, r, defaultHistorySpan)
	if !ok {
		return
	}
	from, to, ok = shiftWindow(from, to, offset)
	if !ok {
		errBadRequest(w, "invalid window")
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
	from, to, ok := rangeWindow(w, r, defaultOutagesSpan)
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
