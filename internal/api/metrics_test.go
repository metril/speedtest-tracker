package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestMetricsRouteGated(t *testing.T) {
	enabled := false
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.MetricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("# HELP speedtest_up\n"))
		})
		d.MetricsEnabled = func() bool { return enabled }
	})
	if rec := do(t, h, http.MethodGet, "/metrics", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled /metrics = %d, want 404", rec.Code)
	}
	enabled = true
	rec := do(t, h, http.MethodGet, "/metrics", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "speedtest_up") {
		t.Fatalf("enabled /metrics = %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodGet, "/healthz", nil); rec.Code != http.StatusOK {
		t.Fatal("/healthz must stay available regardless of the metrics toggle")
	}
}
