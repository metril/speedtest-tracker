package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/fake"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
)

// stubRunner records Enqueue/Cancel calls instead of running tests.
type stubRunner struct {
	lastReq  runner.RunRequest
	nextID   int64
	err      error
	canceled []int64
	cancelOK bool
}

func (s *stubRunner) Enqueue(_ context.Context, req runner.RunRequest) (int64, error) {
	s.lastReq = req
	return s.nextID, s.err
}
func (s *stubRunner) Cancel(id int64) bool {
	s.canceled = append(s.canceled, id)
	return s.cancelOK
}

// stubServers is a fixed Ookla server list.
type stubServers struct{ list []ookla.Server }

func (s stubServers) Servers(context.Context) ([]ookla.Server, error) { return s.list, nil }

func newTestAPI(t *testing.T) (http.Handler, *store.Store, *stubRunner) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := engine.NewRegistry()
	reg.Register(fake.New())
	run := &stubRunner{nextID: 77, cancelOK: true}

	t.Cleanup(func() { reloadHook = nil })

	h := New(Deps{
		Pinger:   db,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Hub:      sse.NewHub(),
		Store:    db,
		Registry: reg,
		Runner:   run,
		ServerList: stubServers{list: []ookla.Server{
			{ID: "1", Name: "Frankfurt Fiber", Location: "Frankfurt", Country: "Germany", Host: "fra.example:8080"},
			{ID: "2", Name: "Init7", Location: "Zurich", Country: "Switzerland", Host: "zrh.example:8080"},
		}},
		ReloadSchedules: func(context.Context) error {
			if reloadHook != nil {
				reloadHook()
			}
			return nil
		},
	})
	return h, db, run
}

func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTargetsCRUDRoutes(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": map[string]any{"download_bps": 1000},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d body=%s", rec.Code, rec.Body)
	}
	var created store.Target
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Name != "home" || created.Lane != "wan" {
		t.Fatalf("created = %+v", created)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/targets", nil)
	var list []store.Target
	json.NewDecoder(rec.Body).Decode(&list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("GET list = %d %+v", rec.Code, list)
	}

	path := "/api/v1/targets/" + itoa(created.ID)
	rec = do(t, h, http.MethodPut, path, map[string]any{
		"name": "renamed", "engine": "fake", "enabled": false, "lane": "lan",
		"options": map[string]any{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d body=%s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, path, nil)
	var got store.Target
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Name != "renamed" || got.Enabled || got.Lane != "lan" {
		t.Errorf("after PUT = %+v", got)
	}

	if rec := do(t, h, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET after delete = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/targets/abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id = %d, want 400", rec.Code)
	}
}

// TestCreateTargetExplicitNullOptionsDefaultsToEmptyObject covers decoding
// an explicit "options": null / "thresholds": null: json.RawMessage stores
// the literal 4-byte "null" for those, which must not be forwarded to the
// engine or persisted as-is; it should behave exactly like the field being
// omitted.
func TestCreateTargetExplicitNullOptionsDefaultsToEmptyObject(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": nil, "thresholds": nil,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST with null options/thresholds = %d body=%s", rec.Code, rec.Body)
	}
	var created store.Target
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if string(created.Options) != "{}" {
		t.Errorf("options = %s, want {}", created.Options)
	}
	if string(created.Thresholds) != "{}" {
		t.Errorf("thresholds = %s, want {}", created.Thresholds)
	}

	path := "/api/v1/targets/" + itoa(created.ID)
	rec = do(t, h, http.MethodPut, path, map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": nil, "thresholds": nil,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT with null options/thresholds = %d body=%s", rec.Code, rec.Body)
	}
	var updated store.Target
	json.NewDecoder(rec.Body).Decode(&updated)
	if string(updated.Options) != "{}" || string(updated.Thresholds) != "{}" {
		t.Errorf("after PUT: options=%s thresholds=%s", updated.Options, updated.Thresholds)
	}
}

func TestCreateTargetValidation(t *testing.T) {
	h, _, _ := newTestAPI(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing name", map[string]any{"engine": "fake"}},
		{"unknown engine", map[string]any{"name": "x", "engine": "nope"}},
		{"bad options", map[string]any{"name": "x", "engine": "fake", "options": map[string]any{"fail": "yes"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/targets", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
			}
			var body errorBody
			json.NewDecoder(rec.Body).Decode(&body)
			if body.Error.Code != "invalid_request" || body.Error.Message == "" {
				t.Errorf("envelope = %+v", body.Error)
			}
		})
	}
}

func TestPostTargetsTestValidatesOptions(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodPost, "/api/v1/targets/test", map[string]any{
		"engine": "fake", "options": map[string]any{"download_bps": 5},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("valid options = %d body=%s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/targets/test", map[string]any{
		"engine": "fake", "options": map[string]any{"fail": "not-a-bool"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid options = %d", rec.Code)
	}
}

func TestRunTargetEnqueues(t *testing.T) {
	h, db, run := newTestAPI(t)
	id, _ := db.CreateTarget(context.Background(), &store.Target{
		Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})

	rec := do(t, h, http.MethodPost, "/api/v1/targets/"+itoa(id)+"/run", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		RunID int64 `json:"run_id"`
	}
	json.NewDecoder(rec.Body).Decode(&body)
	if body.RunID != 77 {
		t.Errorf("run_id = %d, want 77", body.RunID)
	}
	if len(run.lastReq.TargetIDs) != 1 || run.lastReq.TargetIDs[0] != id {
		t.Errorf("enqueued = %+v", run.lastReq)
	}
	if run.lastReq.Trigger != "manual" {
		t.Errorf("trigger = %q, want manual", run.lastReq.Trigger)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/targets/9999/run", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown target run = %d, want 404", rec.Code)
	}
}

func TestTargetLatest(t *testing.T) {
	h, db, _ := newTestAPI(t)
	ctx := context.Background()
	id, _ := db.CreateTarget(ctx, &store.Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})

	if rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(id)+"/latest", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("no results yet = %d, want 404", rec.Code)
	}
	if _, err := db.InsertResult(ctx, &store.Result{
		TargetID: &id, TargetName: "home", Engine: "fake", Status: "ok",
		StartedAt: "2026-09-13T10:00:00.000Z", DownloadBps: 1e8,
	}); err != nil {
		t.Fatal(err)
	}
	rec := do(t, h, http.MethodGet, "/api/v1/targets/"+itoa(id)+"/latest", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var res store.Result
	json.NewDecoder(rec.Body).Decode(&res)
	if res.DownloadBps != 1e8 {
		t.Errorf("latest = %+v", res)
	}
}

func TestOoklaServerSearch(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers", nil)
	var all []ookla.Server
	json.NewDecoder(rec.Body).Decode(&all)
	if rec.Code != http.StatusOK || len(all) != 2 {
		t.Fatalf("all = %d %+v", rec.Code, all)
	}

	for _, q := range []string{"frank", "FRANKFURT", "germany", "fra.example"} {
		rec := do(t, h, http.MethodGet, "/api/v1/ookla/servers?q="+q, nil)
		var got []ookla.Server
		json.NewDecoder(rec.Body).Decode(&got)
		if len(got) != 1 || got[0].ID != "1" {
			t.Errorf("q=%q -> %+v", q, got)
		}
	}
	rec = do(t, h, http.MethodGet, "/api/v1/ookla/servers?q=nowhere", nil)
	var none []ookla.Server
	json.NewDecoder(rec.Body).Decode(&none)
	if len(none) != 0 {
		t.Errorf("no match = %+v", none)
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }
