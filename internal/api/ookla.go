package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
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
	Search(ctx context.Context, q string, limit int) ([]ookla.Server, error)
}

// defaultOoklaSearchLimit bounds the merged result count when the request
// doesn't specify one.
const defaultOoklaSearchLimit = 20

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

	out := []ookla.Server{}
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
		remoteAttempted = true
		remote, err := d.OoklaSearch.Search(r.Context(), q, limit)
		if err != nil {
			remoteErr = err
			if d.Logger != nil {
				d.Logger.Warn("ookla remote search failed", "query", q, "error", err)
			}
		} else {
			for _, s := range remote {
				if seen[s.ID] {
					continue
				}
				seen[s.ID] = true
				out = append(out, s)
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

	writeJSON(w, http.StatusOK, out)
}

func parseOoklaLimit(raw string) int {
	if raw == "" {
		return defaultOoklaSearchLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultOoklaSearchLimit
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
