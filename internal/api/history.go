package api

import (
	"context"
	"net/http"
	"net/url"
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
func rangeWindow(w http.ResponseWriter, q url.Values, defaultSpan time.Duration) (string, string, bool) {
	from, ok := timeQuery(w, q, "from")
	if !ok {
		return "", "", false
	}
	to, ok := timeQuery(w, q, "to")
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
	name := q.Get("range")
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
type historyIn struct {
	ID     int64  `path:"id" minimum:"1" doc:"Resource ID."`
	Range  string `query:"range" enum:"24h,7d,30d" doc:"Named window ending now."`
	Offset string `query:"offset" enum:"0,1" doc:"1 shifts the window back by its own span (previous period)."`
	From   string `query:"from" doc:"Explicit window start (RFC 3339 or unix seconds); use with to instead of range."`
	To     string `query:"to" doc:"Explicit window end (RFC 3339 or unix seconds)."`
}

func (d Deps) targetHistory(ctx context.Context, in *historyIn) (*jsonOut[HistoryResponse], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	id := in.ID
	if _, err := d.Store.GetTarget(ctx, id); err != nil {
		return nil, storeErr(d.Logger, "target", err)
	}
	q := queryOf("range", in.Range, "offset", in.Offset, "from", in.From, "to", in.To)
	c := &errCapture{}
	offset, ok := intQuery(c, q, "offset", 0)
	if !ok {
		return nil, c.err()
	}
	if offset < 0 || offset > 1 {
		return nil, badRequest("offset must be 0 or 1")
	}
	from, to, ok := rangeWindow(c, q, defaultHistorySpan)
	if !ok {
		return nil, c.err()
	}
	from, to, ok = shiftWindow(from, to, offset)
	if !ok {
		return nil, badRequest("invalid window")
	}
	bucket := store.BucketSecondsFor(windowSpan(from, to))
	points, err := d.Store.HistoryBuckets(ctx, id, from, to, bucket)
	if err != nil {
		return nil, internalErr(d.Logger, "history failed", err)
	}
	return &jsonOut[HistoryResponse]{HistoryResponse{
		TargetID: id, From: from, To: to, BucketSeconds: bucket, Points: points,
	}}, nil
}

// HistoryResponse is the GET /targets/{id}/history body.
type HistoryResponse struct {
	BucketSeconds int                  `json:"bucket_seconds"`
	From          string               `json:"from"`
	Points        []store.HistoryPoint `json:"points"`
	TargetID      int64                `json:"target_id"`
	To            string               `json:"to"`
}

// outages answers GET /outages?from&to&gap_seconds.
type outagesIn struct {
	Range      string `query:"range" enum:"24h,7d,30d" doc:"Named window ending now (default 7d)."`
	From       string `query:"from" doc:"Explicit window start (RFC 3339 or unix seconds)."`
	To         string `query:"to" doc:"Explicit window end (RFC 3339 or unix seconds)."`
	GapSeconds string `query:"gap_seconds" doc:"Merge incidents closer together than this (default 1800)."`
}

// OutagesResponse is the GET /outages body.
type OutagesResponse struct {
	From      string           `json:"from"`
	Incidents []store.Incident `json:"incidents"`
	To        string           `json:"to"`
}

func (d Deps) outages(ctx context.Context, in *outagesIn) (*jsonOut[OutagesResponse], error) {
	q := queryOf("range", in.Range, "from", in.From, "to", in.To, "gap_seconds", in.GapSeconds)
	c := &errCapture{}
	from, to, ok := rangeWindow(c, q, defaultOutagesSpan)
	if !ok {
		return nil, c.err()
	}
	gap, ok := intQuery(c, q, "gap_seconds", defaultOutageGapSeconds)
	if !ok {
		return nil, c.err()
	}
	if gap <= 0 {
		return nil, badRequest("gap_seconds must be positive")
	}
	incidents, err := d.Store.Outages(ctx, from, to, gap)
	if err != nil {
		return nil, internalErr(d.Logger, "outages failed", err)
	}
	return &jsonOut[OutagesResponse]{OutagesResponse{From: from, To: to, Incidents: incidents}}, nil
}
