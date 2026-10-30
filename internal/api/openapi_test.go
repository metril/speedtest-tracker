package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

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

	if len(routed) < 40 {
		t.Fatalf("only %d routes discovered; walker broken?", len(routed))
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
