package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

func TestOpenAPIServed(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	for _, path := range []string{"/openapi.json", "/openapi.yaml", "/docs"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if !strings.Contains(rec.Body.String(), `"/api/v1/me"`) {
		t.Fatalf("spec missing /api/v1/me: %s", rec.Body.String())
	}
}

// baseRoutes is the method+path set of the router before the huma
// migration (chi handlers at 78d6156).
var baseRoutes = []string{
	"DELETE /api/v1/queues/{id}",
	"DELETE /api/v1/results/{id}",
	"DELETE /api/v1/runs/{id}",
	"DELETE /api/v1/schedules/{id}",
	"DELETE /api/v1/settings/tokens/{id}",
	"DELETE /api/v1/tags/{id}",
	"DELETE /api/v1/targets/{id}",
	"GET /api/v1/events",
	"GET /api/v1/iperf3/servers",
	"GET /api/v1/me",
	"GET /api/v1/ookla/servers",
	"GET /api/v1/outages",
	"GET /api/v1/queues",
	"GET /api/v1/results",
	"GET /api/v1/results.csv",
	"GET /api/v1/results/{id}",
	"GET /api/v1/runs",
	"GET /api/v1/runs/{id}",
	"GET /api/v1/schedules",
	"GET /api/v1/schedules/{id}",
	"GET /api/v1/schedules/{id}/next",
	"GET /api/v1/settings",
	"GET /api/v1/settings/tokens",
	"GET /api/v1/stats/summary",
	"GET /api/v1/tags",
	"GET /api/v1/targets",
	"GET /api/v1/targets/deleted",
	"GET /api/v1/targets/{id}",
	"GET /api/v1/targets/{id}/history",
	"GET /api/v1/targets/{id}/latest",
	"GET /api/v1/targets/{id}/revisions",
	"GET /auth/logout",
	"GET /auth/mode",
	"GET /auth/oidc/callback",
	"GET /auth/oidc/start",
	"GET /healthz",
	"GET /metrics",
	"POST /api/v1/iperf3/servers/refresh",
	"POST /api/v1/queues",
	"POST /api/v1/results/{id}/reexecute",
	"POST /api/v1/runs",
	"POST /api/v1/schedules",
	"POST /api/v1/schedules/validate",
	"POST /api/v1/schedules/{id}/run",
	"POST /api/v1/settings/test/notify/{channel_id}",
	"POST /api/v1/settings/test/{target}",
	"POST /api/v1/settings/tokens",
	"POST /api/v1/targets",
	"POST /api/v1/targets/deleted/{id}/restore",
	"POST /api/v1/targets/test",
	"POST /api/v1/targets/{id}/revisions/{version}/revert",
	"POST /api/v1/targets/{id}/run",
	"POST /auth/logout",
	"PUT /api/v1/queues/{id}",
	"PUT /api/v1/results/{id}/tags",
	"PUT /api/v1/schedules/{id}",
	"PUT /api/v1/settings",
	"PUT /api/v1/tags/{id}",
	"PUT /api/v1/targets/{id}",
}

// TestSpecCoversRoutes fails when an /api/v1 route is registered on the
// real router without a matching operation in the generated spec, and when
// the spec documents an /api/v1 operation that is not routed.
func TestSpecCoversRoutes(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		d.Settings = s
		d.MetricsHandler = http.NotFoundHandler()
	})

	routed := map[string]bool{}
	err := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		if strings.HasSuffix(route, "/*") {
			return nil // the not-found catch-all
		}
		if strings.HasPrefix(route, "/api/v1") || route == "/healthz" || route == "/metrics" || strings.HasPrefix(route, "/auth/") {
			routed[method+" "+route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	want := map[string]bool{}
	for _, r := range baseRoutes {
		want[r] = true
	}
	for r := range want {
		if !routed[r] {
			t.Errorf("route %s from the original router is missing", r)
		}
		if !documented[r] {
			t.Errorf("route %s from the original router is not in the spec", r)
		}
	}
	for r := range routed {
		if !want[r] {
			t.Errorf("route %s is new relative to the original router; add it to baseRoutes", r)
		}
	}
	for r := range routed {
		if !documented[r] {
			t.Errorf("route %s is not in the OpenAPI spec", r)
		}
	}
	for d := range documented {
		if !routed[d] {
			t.Errorf("spec documents %s but no such route is registered", d)
		}
	}
}

// concretePath fills path parameters with a placeholder.
func concretePath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '{' {
			for p[i] != '}' {
				i++
			}
			b.WriteString("1")
			continue
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

func isPublicRoute(path string) bool {
	return path == "/healthz" || path == "/metrics" || strings.HasPrefix(path, "/auth/")
}

// TestAuthGatesEveryRoute checks that a denied caller gets 401 on every
// non-public route, the spec and docs included, and that a non-admin gets
// 403 on every mutating /api/v1 route.
func TestAuthGatesEveryRoute(t *testing.T) {
	t.Parallel()
	routes := append([]string{"GET /openapi.json", "GET /openapi.yaml", "GET /docs"}, baseRoutes...)

	deny := newParityAPI(t, stubAuth{})
	viewer := newParityAPI(t, stubAuth{&auth.Identity{Mode: "forward_auth", Source: auth.SourceForward}})
	for _, r := range routes {
		method, path, _ := strings.Cut(r, " ")
		path = concretePath(path)
		if !isPublicRoute(path) {
			if rec := send(deny, method, path, "", ""); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s denied = %d, want 401", method, path, rec.Code)
			}
		}
		if strings.HasPrefix(path, "/api/v1/") && method != http.MethodGet {
			if rec := send(viewer, method, path, "application/json", "{}"); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as viewer = %d, want 403", method, path, rec.Code)
			}
		}
	}
}
