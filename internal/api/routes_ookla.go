package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerOokla(api huma.API) {
	register(api, route{Method: http.MethodGet, Path: "/api/v1/ookla/servers", ID: "listOoklaServers", Tag: "servers", Summary: "Search Ookla servers", Errors: []int{502, 503}}, d.listOoklaServers)
}
