package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// stubIperf3Refresher is a scripted Iperf3Refresher.
type stubIperf3Refresher struct {
	fetchedAt time.Time
	count     int
	err       error
	calls     int
}

func (s *stubIperf3Refresher) RefreshNow(context.Context) (time.Time, int, error) {
	s.calls++
	if s.err != nil {
		return time.Time{}, 0, s.err
	}
	return s.fetchedAt, s.count, nil
}

func seedIperf3Servers(t *testing.T, db *store.Store) {
	t.Helper()
	err := db.ReplaceIperf3Servers(context.Background(), []store.Iperf3Server{
		{Host: "iperf.example.net", Port: 5201, Options: "-R,-u", SupportsReverse: true, SupportsUDP: true,
			Country: "DE", Site: "Frankfurt", Provider: "Example Net"},
		{Host: "speed.other.net", Port: 5202, Country: "US", Site: "Denver", Provider: "Other Net"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListIperf3ServersReturnsFetchedAtAndServers(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedIperf3Servers(t, db)

	rec := do(t, h, http.MethodGet, "/api/v1/iperf3/servers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var got struct {
		FetchedAt string               `json:"fetched_at"`
		Servers   []store.Iperf3Server `json:"servers"`
		Total     int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.FetchedAt == "" {
		t.Error("fetched_at is empty")
	}
	if len(got.Servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(got.Servers))
	}
	if got.Total != 2 {
		t.Errorf("total = %d, want 2", got.Total)
	}
}

func TestListIperf3ServersFiltersByQuery(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedIperf3Servers(t, db)

	rec := do(t, h, http.MethodGet, "/api/v1/iperf3/servers?q=denver", nil)
	var got struct {
		Servers []store.Iperf3Server `json:"servers"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 1 || got.Servers[0].Host != "speed.other.net" {
		t.Fatalf("got = %+v", got.Servers)
	}
}

func TestListIperf3ServersDefaultAndCapLimit(t *testing.T) {
	h, db, _ := newTestAPI(t)
	servers := make([]store.Iperf3Server, 0, 300)
	for i := 0; i < 300; i++ {
		servers = append(servers, store.Iperf3Server{Host: "many.example.net", Port: 5201 + i})
	}
	if err := db.ReplaceIperf3Servers(context.Background(), servers); err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodGet, "/api/v1/iperf3/servers", nil)
	var got struct {
		Servers []store.Iperf3Server `json:"servers"`
	}
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Servers) != 50 {
		t.Fatalf("default limit: got %d, want 50", len(got.Servers))
	}

	rec = do(t, h, http.MethodGet, "/api/v1/iperf3/servers?limit=500", nil)
	got.Servers = nil
	json.NewDecoder(rec.Body).Decode(&got)
	if len(got.Servers) != 200 {
		t.Fatalf("capped limit: got %d, want 200", len(got.Servers))
	}
}

func TestRefreshIperf3ServersCallsRefresherAndReturnsResult(t *testing.T) {
	fetchedAt := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	refresher := &stubIperf3Refresher{fetchedAt: fetchedAt, count: 7}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.Iperf3 = refresher })

	rec := do(t, h, http.MethodPost, "/api/v1/iperf3/servers/refresh", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var got struct {
		FetchedAt time.Time `json:"fetched_at"`
		Count     int       `json:"count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 7 || !got.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("got = %+v", got)
	}
	if refresher.calls != 1 {
		t.Errorf("refresher called %d times, want 1", refresher.calls)
	}
}

func TestRefreshIperf3ServersPropagatesError(t *testing.T) {
	refresher := &stubIperf3Refresher{err: context.DeadlineExceeded}
	h, _, _ := newTestAPIWith(t, func(d *Deps) { d.Iperf3 = refresher })

	rec := do(t, h, http.MethodPost, "/api/v1/iperf3/servers/refresh", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

func TestRefreshIperf3ServersUnavailableWithoutRefresher(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodPost, "/api/v1/iperf3/servers/refresh", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestRefreshIperf3ServersRequiresAdmin(t *testing.T) {
	refresher := &stubIperf3Refresher{count: 1}
	var mw *auth.Middleware
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		d.Iperf3 = refresher
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		d.Settings = s
		mw = auth.New(nil, storeTokenLookup{db: d.Store}, time.Now)
		cfg := settings.Auth{Mode: settings.AuthModeForward, UserHeader: "Remote-User",
			GroupsHeader: "Remote-Groups", GroupsSeparator: ",", AdminGroup: "admins",
			TrustedProxies: []string{"192.0.2.0/24"}}
		if err := mw.Configure(cfg); err != nil {
			t.Fatal(err)
		}
		d.Auth = authAdapter{mw: mw, mode: cfg.Mode}
	})
	t.Cleanup(func() {
		if mw != nil {
			mw.Close()
		}
	})

	req := jsonRequest(t, http.MethodPost, "/api/v1/iperf3/servers/refresh", nil)
	req.RemoteAddr = "192.0.2.9:1"
	req.Header.Set("Remote-User", "bob")
	req.Header.Set("Remote-Groups", "users")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST refresh as non-admin = %d %s, want 403", rec.Code, rec.Body)
	}
	if refresher.calls != 0 {
		t.Errorf("refresher called %d times, want 0 for a forbidden request", refresher.calls)
	}
}
