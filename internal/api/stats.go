package api

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// summaryTTL is how long a computed summary is reused. The dashboard
// polls; recomputing aggregates on every poll is wasted work.
const summaryTTL = 30 * time.Second

// maxSummaryCacheEntries bounds the cache regardless of TTL expiry, since
// arbitrary from/to pairs would otherwise let a caller grow it without
// limit. Named ranges plus the default keep real usage well under this.
const maxSummaryCacheEntries = 64

type summaryEntry struct {
	body *store.SummaryStats
	at   time.Time
}

// summaryCache memoises /stats/summary bodies per window key, evicting the
// oldest-inserted entry once it holds more than maxSummaryCacheEntries.
type summaryCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]summaryEntry
	order   []string         // insertion order, oldest first, for eviction
	now     func() time.Time // swapped in tests
}

func newSummaryCache(ttl time.Duration) *summaryCache {
	return &summaryCache{ttl: ttl, entries: map[string]summaryEntry{}, now: time.Now}
}

func (c *summaryCache) get(key string) (*store.SummaryStats, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.at) > c.ttl {
		return nil, false
	}
	return e.body, true
}

func (c *summaryCache) set(key string, body *store.SummaryStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = summaryEntry{body: body, at: c.now()}
	for len(c.entries) > maxSummaryCacheEntries {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
}

// slaKeyPart renders sla into a cache-key fragment that changes whenever
// the resolved plan does, so a settings change invalidates the cache
// immediately instead of waiting out summaryTTL.
func slaKeyPart(sla store.SLAPlan) string {
	f := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'g', -1, 64)
	}
	return f(sla.DownloadMbps) + "," + f(sla.UploadMbps) + "," + f(sla.TolerancePct)
}

// statsSummary answers GET /stats/summary?range=[&offset=], serving a
// cached body for up to summaryTTL. offset=1 shifts the resolved window
// back by its own span, giving the caller the immediately preceding
// period of equal length for a previous-period comparison.
type statsSummaryIn struct {
	Range  string `query:"range" enum:"24h,7d,30d" doc:"Named window ending now (default 24h)."`
	Offset string `query:"offset" enum:"0,1" doc:"1 shifts the window back by its own span (previous period)."`
	From   string `query:"from" doc:"Not accepted; present only to reject explicit windows."`
	To     string `query:"to" doc:"Not accepted; present only to reject explicit windows."`
}

type statsSummaryOut struct {
	CacheControl string `header:"Cache-Control"`
	Body         store.SummaryStats
}

func (d Deps) statsSummary(ctx context.Context, in *statsSummaryIn) (*statsSummaryOut, error) {
	if in.From != "" || in.To != "" {
		return nil, badRequest("stats/summary only accepts range=24h|7d|30d, not an explicit from/to pair")
	}
	q := queryOf("range", in.Range, "offset", in.Offset)
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

	// The general SLA plan is resolved before the cache lookup (not just
	// before the miss-path Store.Summary call) and folded into the cache
	// key, so a plan change via PUT /settings is reflected immediately
	// instead of possibly serving a stale sla_compliance for up to
	// summaryTTL.
	sla := store.SLAPlan{}
	if d.Settings != nil {
		g, err := d.Settings.General(ctx)
		if err != nil {
			return nil, internalErr(d.Logger, "load general settings", err)
		}
		sla = store.SLAPlan{DownloadMbps: g.SLADownloadMbps, UploadMbps: g.SLAUploadMbps, TolerancePct: g.SLATolerancePct}
	}
	key := from + "|" + to + "|" + strconv.Itoa(offset) + "|" + slaKeyPart(sla)
	const cacheControl = "private, max-age=30"
	if body, hit := d.summary.get(key); hit {
		if d.Metrics != nil {
			d.Metrics.SummaryCacheHit()
		}
		return &statsSummaryOut{CacheControl: cacheControl, Body: *body}, nil
	}
	if d.Metrics != nil {
		d.Metrics.SummaryCacheMiss()
	}
	stats, err := d.Store.Summary(ctx, from, to, sla)
	if err != nil {
		return nil, internalErr(d.Logger, "summary failed", err)
	}
	d.summary.set(key, stats)
	return &statsSummaryOut{CacheControl: cacheControl, Body: *stats}, nil
}
