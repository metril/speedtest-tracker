package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/iperf3list"
	"github.com/metril/speedtest-tracker/internal/store"
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
type listIperf3In struct {
	Q     string `query:"q" doc:"Free-text search."`
	Limit string `query:"limit" doc:"Maximum servers to return (default 50, max 200)."`
}

// Iperf3ServersPage is the GET /iperf3/servers body.
type Iperf3ServersPage struct {
	FetchedAt string               `json:"fetched_at"`
	Servers   []store.Iperf3Server `json:"servers"`
	Total     int                  `json:"total"`
}

// Iperf3Refreshed is the POST /iperf3/servers/refresh body.
type Iperf3Refreshed struct {
	Count     int       `json:"count"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (d Deps) listIperf3Servers(ctx context.Context, in *listIperf3In) (*jsonOut[Iperf3ServersPage], error) {
	q := strings.TrimSpace(in.Q)
	limit := parseIperf3Limit(in.Limit)

	servers, err := d.Store.SearchIperf3Servers(ctx, q, limit)
	if err != nil {
		return nil, internalErr(d.Logger, "search iperf3 servers", err)
	}
	fetchedAt, err := d.Store.Iperf3ServersFetchedAt(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "read iperf3 servers fetched_at", err)
	}
	total, err := d.Store.CountIperf3Servers(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "count iperf3 servers", err)
	}
	return &jsonOut[Iperf3ServersPage]{Iperf3ServersPage{FetchedAt: fetchedAt, Servers: servers, Total: total}}, nil
}

// refreshIperf3Servers answers POST /iperf3/servers/refresh, admin-gated
// like the other write routes. The refresh runs synchronously, using the
// request's own context, so the response reflects the new count.
func (d Deps) refreshIperf3Servers(ctx context.Context, _ *struct{}) (*jsonOut[Iperf3Refreshed], error) {
	if !requestIsAdmin(ctx) {
		return nil, forbidden("admin access required")
	}
	if d.Iperf3 == nil {
		return nil, apiErr(http.StatusServiceUnavailable, "unavailable", "iperf3 server list refresh not configured")
	}
	fetchedAt, count, err := d.Iperf3.RefreshNow(ctx)
	if err != nil {
		if errors.Is(err, iperf3list.ErrDisabled) {
			return nil, conflict("disabled",
				"iperf3 server list refresh is disabled: set Engines > iperf3 server list in Settings to enable it")
		}
		d.Logger.Warn("iperf3 server list refresh failed", "error", err)
		return nil, apiErr(http.StatusBadGateway, "refresh_failed", "refreshing the iperf3 server list failed; see server logs")
	}
	return &jsonOut[Iperf3Refreshed]{Iperf3Refreshed{Count: count, FetchedAt: fetchedAt}}, nil
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
