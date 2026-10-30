package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
