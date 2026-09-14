package api

import (
	"net/http"
	"testing"
)

func TestListEndpointsAnswer304OnMatchingETag(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedResults(t, db, 2)

	for _, path := range []string{"/api/v1/results", "/api/v1/targets", "/api/v1/schedules", "/api/v1/runs"} {
		first := do(t, h, http.MethodGet, path, nil)
		if first.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", path, first.Code)
		}
		tag := first.Header().Get("ETag")
		if tag == "" {
			t.Fatalf("%s: no ETag header", path)
		}
		req := newRequest(t, http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", tag)
		second := serve(t, h, req)
		if second.Code != http.StatusNotModified {
			t.Errorf("%s: revalidation status = %d, want 304", path, second.Code)
		}
		if second.Body.Len() != 0 {
			t.Errorf("%s: 304 carried a body", path)
		}
	}
}

func TestETagChangesWhenDataChanges(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedResults(t, db, 1)
	before := do(t, h, http.MethodGet, "/api/v1/results", nil).Header().Get("ETag")
	seedResults(t, db, 1)
	after := do(t, h, http.MethodGet, "/api/v1/results", nil).Header().Get("ETag")
	if before == after {
		t.Fatalf("ETag unchanged (%s) after inserting a result", before)
	}
}
