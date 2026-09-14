package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/ooklaweb"
)

// ServerLister supplies the local Ookla server list (`speedtest -L`,
// cached upstream).
type ServerLister interface {
	Servers(ctx context.Context) ([]ookla.Server, error)
}

// ServerSearcher supplies a wider, network-backed Ookla server search
// (speedtest.net's unofficial search API, widened via geocoding — see
// internal/ooklaweb). Optional: nil means only the local list is served.
type ServerSearcher interface {
	Search(ctx context.Context, req ooklaweb.SearchRequest) (ooklaweb.SearchResult, error)
}

// defaultOoklaSearchLimit bounds the merged result count when the request
// doesn't specify one; maxOoklaSearchLimit caps an explicit one, mirroring
// parseIperf3Limit.
const (
	defaultOoklaSearchLimit = 20
	maxOoklaSearchLimit     = 200
)

// RateLimiter gates remote Ookla searches. Allow reports whether a remote
// search may proceed right now, consuming a token if so.
type RateLimiter interface {
	Allow() bool
}

// ooklaLimiterBurst and ooklaLimiterRefillPerSecond size the default
// per-process token-bucket limiter NewOoklaLimiter builds: a burst of 10
// remote searches, refilling at 1/s — generous for interactive typing in
// the server picker, but enough to stop a runaway client (or several)
// from hammering speedtest.net.
const (
	ooklaLimiterBurst           = 10
	ooklaLimiterRefillPerSecond = 1.0
)

// tokenBucketLimiter is a small, dependency-free token-bucket rate
// limiter (a stand-in for golang.org/x/time/rate.Limiter, which is not a
// project dependency). The zero value is not usable; construct with
// NewOoklaLimiter.
type tokenBucketLimiter struct {
	mu           sync.Mutex
	tokens       float64
	burst        float64
	refillPerSec float64
	last         time.Time
	now          func() time.Time
}

// NewOoklaLimiter returns a RateLimiter sized for GET /ookla/servers'
// remote search: burst 10, refill 1/s. Intended to be constructed once
// per process and shared across requests via Deps.OoklaLimiter.
func NewOoklaLimiter() RateLimiter {
	return &tokenBucketLimiter{
		tokens: ooklaLimiterBurst, burst: ooklaLimiterBurst, refillPerSec: ooklaLimiterRefillPerSecond,
		now: time.Now,
	}
}

// Allow reports whether a call may proceed right now, consuming one token
// if so.
func (l *tokenBucketLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.last.IsZero() {
		if elapsed := now.Sub(l.last).Seconds(); elapsed > 0 {
			l.tokens += elapsed * l.refillPerSec
			if l.tokens > l.burst {
				l.tokens = l.burst
			}
		}
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// listOoklaServers answers GET /ookla/servers?q=&limit=. Local (`speedtest
// -L`, filtered by matchesServer) and remote (OoklaSearch, when q is set)
// are independent sources: either can fail without taking the other down,
// and hits are merged deduped by ID. A failing source is logged at warn.
// The only hard failure (502) is a non-empty query where local errored and
// remote either wasn't available to fall back on or also errored — with no
// query, remote is never attempted, so a local failure there just yields an
// empty list. A 503 "unavailable" is reserved for neither source being
// configured at all.
func (d Deps) listOoklaServers(w http.ResponseWriter, r *http.Request) {
	if d.ServerList == nil && d.OoklaSearch == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "ookla server list not configured")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	lowerQ := strings.ToLower(q)
	limit := parseOoklaLimit(r.URL.Query().Get("limit"))

	country := ""
	if raw := strings.TrimSpace(r.URL.Query().Get("country")); raw != "" {
		if len(raw) != 2 || !isASCIILetters(raw) {
			errBadRequest(w, "country must be a 2-letter country code")
			return
		}
		country = strings.ToLower(raw)
	}

	out := []ookla.Server{}
	near := ""
	seen := make(map[string]bool)

	var localErr error
	if d.ServerList != nil {
		servers, err := d.ServerList.Servers(r.Context())
		if err != nil {
			localErr = err
			if d.Logger != nil {
				d.Logger.Warn("ookla local server list failed", "error", err)
			}
		} else {
			for _, s := range servers {
				if lowerQ == "" || matchesServer(s, lowerQ) {
					out = append(out, s)
					seen[s.ID] = true
				}
			}
		}
	}

	var remoteAttempted bool
	var remoteErr error
	if q != "" && d.OoklaSearch != nil {
		if d.OoklaLimiter != nil && !d.OoklaLimiter.Allow() {
			// Upstream protection: serve local results only, never error
			// the request just because the limiter denied this remote
			// search.
			if d.Logger != nil {
				d.Logger.Debug("ookla remote search rate limited, serving local results only", "query", q)
			}
		} else {
			remoteAttempted = true
			remote, err := d.OoklaSearch.Search(r.Context(), ooklaweb.SearchRequest{Q: q, Country: country, Limit: limit})
			if err != nil {
				remoteErr = err
				if d.Logger != nil {
					d.Logger.Warn("ookla remote search failed", "query", q, "error", err)
				}
			} else {
				near = remote.Near
				for _, s := range remote.Servers {
					if seen[s.ID] {
						continue
					}
					seen[s.ID] = true
					out = append(out, s)
				}
			}
		}
	}

	if q != "" && localErr != nil && (!remoteAttempted || remoteErr != nil) {
		writeError(w, http.StatusBadGateway, "server_list_failed", localErr.Error())
		return
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"servers": out, "near": near})
}

func parseOoklaLimit(raw string) int {
	if raw == "" {
		return defaultOoklaSearchLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultOoklaSearchLimit
	}
	if n > maxOoklaSearchLimit {
		return maxOoklaSearchLimit
	}
	return n
}

func matchesServer(s ookla.Server, q string) bool {
	for _, field := range []string{s.Name, s.Location, s.Country, s.Host} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

// isASCIILetters reports whether s consists only of ASCII letters (upper
// or lower case).
func isASCIILetters(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
