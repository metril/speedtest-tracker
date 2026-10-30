package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerOutages(api huma.API) {
	register(api, route{Method: http.MethodGet, Path: "/api/v1/outages", ID: "listOutages", Tag: "stats", Summary: "Failure and skipped-run incidents in a window"}, d.outages)
}
