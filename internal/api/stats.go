package api

import (
	"encoding/json"
	"net/http"
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
	body []byte
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

func (c *summaryCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.at) > c.ttl {
		return nil, false
	}
	return e.body, true
}

func (c *summaryCache) set(key string, body []byte) {
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
func (d Deps) statsSummary(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("from") != "" || r.URL.Query().Get("to") != "" {
		errBadRequest(w, "stats/summary only accepts range=24h|7d|30d, not an explicit from/to pair")
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

	// The general SLA plan is resolved before the cache lookup (not just
	// before the miss-path Store.Summary call) and folded into the cache
	// key, so a plan change via PUT /settings is reflected immediately
	// instead of possibly serving a stale sla_compliance for up to
	// summaryTTL.
	sla := store.SLAPlan{}
	if d.Settings != nil {
		g, err := d.Settings.General(r.Context())
		if err != nil {
			internalError(w, d.Logger, "load general settings", err)
			return
		}
		sla = store.SLAPlan{DownloadMbps: g.SLADownloadMbps, UploadMbps: g.SLAUploadMbps, TolerancePct: g.SLATolerancePct}
	}
	key := from + "|" + to + "|" + strconv.Itoa(offset) + "|" + slaKeyPart(sla)
	w.Header().Set("Cache-Control", "max-age=30")
	if body, hit := d.summary.get(key); hit {
		if d.Metrics != nil {
			d.Metrics.SummaryCacheHit()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		return
	}
	if d.Metrics != nil {
		d.Metrics.SummaryCacheMiss()
	}
	stats, err := d.Store.Summary(r.Context(), from, to, sla)
	if err != nil {
		internalError(w, d.Logger, "summary failed", err)
		return
	}
	body, err := json.Marshal(stats)
	if err != nil {
		internalError(w, d.Logger, "summary encode failed", err)
		return
	}
	d.summary.set(key, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
