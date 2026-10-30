package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

func init() { registerSpec(statsSpecs) }

func statsSpecs() []opSpec {
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/stats/summary", ID: "getStatsSummary", Tag: "stats", Summary: "Per-target and overall summary statistics",
			Params: []p{rangeParam, offsetParam}, Out: store.SummaryStats{}},
	}
}
