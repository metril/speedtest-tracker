package api

import "net/http"

// AuthModeResponse is the GET /auth/mode body.
type AuthModeResponse = authModeResponse

// Routes that are not plain JSON request/response (a stream, a download,
// browser redirects, plain text) stay on hand-written chi handlers and are
// documented here only.

func init() { registerSpec(miscSpecs) }

func miscSpecs() []opSpec {
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/events", ID: "streamEvents", Tag: "events",
			Summary: "Server-sent events stream of run and result updates",
			Status:  200, Out: "", OutType: "text/event-stream"},
		{Method: http.MethodGet, Path: "/api/v1/results.csv", ID: "exportResultsCsv", Tag: "results",
			Summary: "Stream matching results as a CSV download",
			Params:  resultFilterParams, Out: "", OutType: "text/csv"},
		{Method: http.MethodGet, Path: "/healthz", ID: "getHealth", Tag: "ops", NoAuth: true,
			Summary: "Liveness and database health", Out: "", OutType: "text/plain", Errors: []int{503}},
		{Method: http.MethodGet, Path: "/metrics", ID: "getMetrics", Tag: "ops",
			Summary: "Prometheus metrics (when enabled in settings)", Out: "", OutType: "text/plain"},
		{Method: http.MethodGet, Path: "/auth/mode", ID: "getAuthMode", Tag: "auth", NoAuth: true,
			Summary: "Configured auth mode (public; backs the login page)", Out: AuthModeResponse{}},
		{Method: http.MethodGet, Path: "/auth/oidc/start", ID: "oidcStart", Tag: "auth", NoAuth: true,
			Summary: "Begin the OIDC login flow (redirects to the provider)", Status: 302,
			Params: []p{{Name: "return_to", Query: true, Type: "string", Doc: "Same-origin path to return to after login."}}},
		{Method: http.MethodGet, Path: "/auth/oidc/callback", ID: "oidcCallback", Tag: "auth", NoAuth: true,
			Summary: "OIDC redirect target; sets the session cookie", Status: 302,
			Params: []p{{Name: "code", Query: true, Type: "string"}, {Name: "state", Query: true, Type: "string"}, {Name: "error", Query: true, Type: "string"}}},
		{Method: http.MethodGet, Path: "/auth/logout", ID: "logoutGet", Tag: "auth", NoAuth: true, Summary: "End the session"},
		{Method: http.MethodPost, Path: "/auth/logout", ID: "logout", Tag: "auth", NoAuth: true, Summary: "End the session"},
	}
}
