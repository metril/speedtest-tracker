package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Iperf3Refresher triggers an immediate refresh of the cached public
// iperf3 server list. It is internal/iperf3list.Refresher in production.
type Iperf3Refresher interface {
	RefreshNow(ctx context.Context) (fetchedAt time.Time, count int, err error)
}

// defaultIperf3Limit and maxIperf3Limit bound GET /iperf3/servers?limit=.
const (
	defaultIperf3Limit = 50
	maxIperf3Limit     = 200
)

// listIperf3Servers answers GET /iperf3/servers?q=&limit=, searching the
// cached public iperf3 server list.
func (d Deps) listIperf3Servers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseIperf3Limit(r.URL.Query().Get("limit"))

	servers, err := d.Store.SearchIperf3Servers(r.Context(), q, limit)
	if err != nil {
		internalError(w, d.Logger, "search iperf3 servers", err)
		return
	}
	fetchedAt, err := d.Store.Iperf3ServersFetchedAt(r.Context())
	if err != nil {
		internalError(w, d.Logger, "read iperf3 servers fetched_at", err)
		return
	}
	total, err := d.Store.CountIperf3Servers(r.Context())
	if err != nil {
		internalError(w, d.Logger, "count iperf3 servers", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"fetched_at": fetchedAt,
		"servers":    servers,
		"total":      total,
	})
}

// refreshIperf3Servers answers POST /iperf3/servers/refresh, admin-gated
// like the other write routes. The refresh runs synchronously, using the
// request's own context, so the response reflects the new count.
func (d Deps) refreshIperf3Servers(w http.ResponseWriter, r *http.Request) {
	if !requestIsAdmin(r) {
		errForbidden(w, "admin access required")
		return
	}
	if d.Iperf3 == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "iperf3 server list refresh not configured")
		return
	}
	fetchedAt, count, err := d.Iperf3.RefreshNow(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "refresh_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fetched_at": fetchedAt,
		"count":      count,
	})
}

func parseIperf3Limit(raw string) int {
	if raw == "" {
		return defaultIperf3Limit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultIperf3Limit
	}
	if n > maxIperf3Limit {
		return maxIperf3Limit
	}
	return n
}
