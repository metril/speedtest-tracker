package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// OutagesResponse is the GET /outages body.
type OutagesResponse struct {
	From      string           `json:"from"`
	To        string           `json:"to"`
	Incidents []store.Incident `json:"incidents"`
}

func init() { registerSpec(statsSpecs) }

func statsSpecs() []opSpec {
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/outages", ID: "listOutages", Tag: "stats", Summary: "Failure and skipped-run incidents in a window",
			Params: []p{rangeParam,
				{Name: "from", Query: true, Type: "string", Doc: "Explicit window start (RFC 3339)."},
				{Name: "to", Query: true, Type: "string", Doc: "Explicit window end (RFC 3339)."},
				{Name: "gap_seconds", Query: true, Doc: "Merge incidents closer together than this."}},
			Out: OutagesResponse{}},
		{Method: http.MethodGet, Path: "/api/v1/stats/summary", ID: "getStatsSummary", Tag: "stats", Summary: "Per-target and overall summary statistics",
			Params: []p{rangeParam, offsetParam}, Out: store.SummaryStats{}},
	}
}
