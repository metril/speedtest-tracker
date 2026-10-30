package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerStats(api huma.API) {
	register(api, route{Method: http.MethodGet, Path: "/api/v1/stats/summary", ID: "getStatsSummary", Tag: "stats", Summary: "Per-target and overall summary statistics"}, d.statsSummary)
}
