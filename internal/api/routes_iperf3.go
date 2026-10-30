package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerIperf3(api huma.API) {
	register(api, route{Method: http.MethodGet, Path: "/api/v1/iperf3/servers", ID: "listIperf3Servers", Tag: "servers", Summary: "Search the cached public iperf3 server list"}, d.listIperf3Servers)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/iperf3/servers/refresh", ID: "refreshIperf3Servers", Tag: "servers", Summary: "Refresh the public iperf3 server list now", Errors: []int{409, 502, 503}}, d.refreshIperf3Servers)
}
