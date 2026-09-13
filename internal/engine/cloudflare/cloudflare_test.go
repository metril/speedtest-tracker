package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine"
)

// testServer serves the three Cloudflare speed-test endpoints locally.
func testServer(t *testing.T) (*httptest.Server, *int64, *int64) {
	t.Helper()
	var downs, ups int64
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn-cgi/trace", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "fl=10f20\nh=speed.cloudflare.com\nip=203.0.113.9\nts=1760000000.000\ncolo=FRA\nloc=DE\n")
	})
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&downs, 1)
		n, err := strconv.Atoi(r.URL.Query().Get("bytes"))
		if err != nil || n < 0 {
			http.Error(w, "bad bytes", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		buf := make([]byte, 4096)
		for written := 0; written < n; {
			chunk := len(buf)
			if n-written < chunk {
				chunk = n - written
			}
			if _, err := w.Write(buf[:chunk]); err != nil {
				return
			}
			written += chunk
		}
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&ups, 1)
		n, _ := io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Length", "0")
		_ = n
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &downs, &ups
}

func TestRunAgainstHTTPTestServer(t *testing.T) {
	srv, downs, ups := testServer(t)
	opts, _ := json.Marshal(Options{
		DownloadSizes:  []int{100_000, 200_000},
		UploadSizes:    []int{50_000, 100_000},
		LatencySamples: 3,
		BaseURL:        srv.URL,
	})

	var events []engine.Progress
	res, err := New(srv.Client()).Run(context.Background(), opts, func(p engine.Progress) {
		events = append(events, p)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.DownloadBps <= 0 || res.UploadBps <= 0 {
		t.Errorf("throughput = %v / %v, want > 0", res.DownloadBps, res.UploadBps)
	}
	if res.PingMs <= 0 {
		t.Errorf("PingMs = %v, want > 0", res.PingMs)
	}
	if res.BytesDown != 300_000 || res.BytesUp != 150_000 {
		t.Errorf("bytes = %d / %d, want 300000 / 150000", res.BytesDown, res.BytesUp)
	}
	if res.ExternalIP != "203.0.113.9" {
		t.Errorf("ExternalIP = %q, want 203.0.113.9", res.ExternalIP)
	}
	if res.ServerName != "FRA" {
		t.Errorf("ServerName = %q, want the colo FRA", res.ServerName)
	}
	if got := atomic.LoadInt64(downs); got != 2 {
		t.Errorf("download requests = %d, want 2", got)
	}
	if got := atomic.LoadInt64(ups); got != 2 {
		t.Errorf("upload requests = %d, want 2", got)
	}

	phases := map[engine.Phase]int{}
	for _, e := range events {
		phases[e.Phase]++
	}
	for _, p := range []engine.Phase{engine.PhaseConnecting, engine.PhasePing, engine.PhaseDownload, engine.PhaseUpload, engine.PhaseDone} {
		if phases[p] == 0 {
			t.Errorf("no %q progress events", p)
		}
	}
	if last := events[len(events)-1]; last.Phase != engine.PhaseDone || last.Progress != 1 {
		t.Errorf("last event = %+v", last)
	}
}

func TestRunServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	opts, _ := json.Marshal(Options{BaseURL: srv.URL, LatencySamples: 1, DownloadSizes: []int{1000}, UploadSizes: []int{1000}})
	if _, err := New(srv.Client()).Run(context.Background(), opts, nil); err == nil {
		t.Fatal("want error")
	}
}

func TestRunHonoursContextCancel(t *testing.T) {
	blocked := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn-cgi/trace", func(w http.ResponseWriter, r *http.Request) {
		close(blocked)
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts, _ := json.Marshal(Options{BaseURL: srv.URL, LatencySamples: 1})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-blocked
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := New(srv.Client()).Run(ctx, opts, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancellation")
	}
}

func TestValidateAndDefaults(t *testing.T) {
	e := New(nil)
	if err := e.Validate(json.RawMessage(`{"latency_samples":0}`)); err != nil {
		t.Errorf("zero samples should fall back to default: %v", err)
	}
	if err := e.Validate(json.RawMessage(`{"latency_samples":-1}`)); err == nil {
		t.Error("want error for negative latency_samples")
	}
	if err := e.Validate(json.RawMessage(`{"base_url":"://bad"}`)); err == nil {
		t.Error("want error for invalid base_url")
	}
	if err := e.Validate(json.RawMessage(`{"download_sizes":[0]}`)); err == nil {
		t.Error("want error for non-positive size")
	}
	o, err := parseOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.BaseURL != "https://speed.cloudflare.com" || o.LatencySamples != 10 {
		t.Errorf("defaults = %+v", o)
	}
	if len(o.DownloadSizes) != 4 || o.DownloadSizes[3] != 100_000_000 {
		t.Errorf("download defaults = %v", o.DownloadSizes)
	}
	if len(o.UploadSizes) != 3 || o.UploadSizes[0] != 100_000 {
		t.Errorf("upload defaults = %v", o.UploadSizes)
	}
}

func TestPercentile(t *testing.T) {
	v := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := percentile(v, 0.9); got != 90 {
		t.Errorf("p90 = %v, want 90", got)
	}
	if got := percentile([]float64{5}, 0.9); got != 5 {
		t.Errorf("p90 of single = %v, want 5", got)
	}
	if got := percentile(nil, 0.9); got != 0 {
		t.Errorf("p90 of empty = %v, want 0", got)
	}
}
