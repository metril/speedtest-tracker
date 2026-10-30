package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// Iperf3ServersPage is the GET /iperf3/servers body.
type Iperf3ServersPage struct {
	FetchedAt string               `json:"fetched_at"`
	Servers   []store.Iperf3Server `json:"servers"`
	Total     int                  `json:"total"`
}

// Iperf3Refreshed is the POST /iperf3/servers/refresh body.
type Iperf3Refreshed struct {
	FetchedAt string `json:"fetched_at"`
	Count     int    `json:"count"`
}

func init() { registerSpec(serverSpecs) }

func serverSpecs() []opSpec {
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/iperf3/servers", ID: "listIperf3Servers", Tag: "servers", Summary: "Search the cached public iperf3 server list",
			Params: []p{{Name: "q", Query: true, Type: "string", Doc: "Free-text search."}, limitParam}, Out: Iperf3ServersPage{}},
		{Method: http.MethodPost, Path: "/api/v1/iperf3/servers/refresh", ID: "refreshIperf3Servers", Tag: "servers", Summary: "Refresh the public iperf3 server list now",
			Out: Iperf3Refreshed{}, Errors: []int{409, 502, 503}},
	}
}
