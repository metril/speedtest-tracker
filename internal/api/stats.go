package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
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
	if offset > 1 {
		errBadRequest(w, "offset must be 0 or 1")
		return
	}
	from, to, ok := rangeWindow(w, r, defaultHistorySpan)
	if !ok {
		return
	}
	if offset == 1 {
		span := windowSpan(from, to)
		f, err := time.Parse(dbTimeFormat, from)
		if err != nil {
			errBadRequest(w, "invalid window")
			return
		}
		to = from
		from = f.Add(-span).Format(dbTimeFormat)
	}
	key := from + "|" + to + "|" + r.URL.Query().Get("offset")
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
	stats, err := d.Store.Summary(r.Context(), from, to)
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
