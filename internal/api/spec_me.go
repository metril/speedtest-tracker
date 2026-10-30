package api

import "net/http"

func init() { registerSpec(meSpecs) }

func meSpecs() []opSpec {
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/me", ID: "getMe", Tag: "me",
			Summary: "Resolved identity of the caller", Out: meResponse{}},
	}
}
