package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

// doJSON is an alias for do, used by tests that send/inspect JSON bodies.
func doJSON(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	return do(t, h, method, path, body)
}

// reloadHook lets tests count scheduler reloads triggered by handlers. The
// handler built by newTestAPI calls whatever function is installed here.
var reloadHook func()

// withReload installs fn as the reload callback for the handler under test.
func withReload(fn func()) { reloadHook = fn }

// createTestTarget posts a target and returns its id.
func createTestTarget(t *testing.T, h http.Handler, name string) int64 {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/v1/targets",
		map[string]any{"name": name, "engine": "fake", "enabled": true, "lane": "wan", "options": map[string]any{}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create target: status %d body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func TestCreateScheduleReturnsScheduleAndReloads(t *testing.T) {
	h, _, _ := newTestAPI(t)
	reloads := 0
	withReload(func() { reloads++ })
	id := createTestTarget(t, h, "t1")

	rec := do(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "nightly", "cron": "0 3 * * *", "enabled": true,
		"timezone": "Europe/Zurich", "target_ids": []int64{id}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Schedule struct {
			ID        int64   `json:"id"`
			Name      string  `json:"name"`
			TargetIDs []int64 `json:"target_ids"`
			NextRun   string  `json:"next_run"`
		} `json:"schedule"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schedule.ID == 0 || body.Schedule.Name != "nightly" || len(body.Schedule.TargetIDs) != 1 {
		t.Fatalf("schedule = %+v", body.Schedule)
	}
	if body.Schedule.NextRun == "" {
		t.Fatal("next_run is empty for an enabled schedule")
	}
	if body.Warnings == nil {
		t.Fatal("warnings must be [] not null")
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want 1", reloads)
	}
}

func TestCreateScheduleRejectsBadCronTimezoneAndTargets(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t1")
	cases := []struct {
		name string
		body map[string]any
	}{
		{"bad cron", map[string]any{"name": "a", "cron": "nonsense", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}}},
		{"bad timezone", map[string]any{"name": "a", "cron": "@hourly", "enabled": true, "timezone": "Mars/Olympus", "target_ids": []int64{id}}},
		{"no targets", map[string]any{"name": "a", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{}}},
		{"unknown target", map[string]any{"name": "a", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{9999}}},
		{"blank name", map[string]any{"name": "  ", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/schedules", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestCreateScheduleDuplicateTargetIDsIs400 checks that repeated target ids
// are reported distinctly from an unknown target id.
func TestCreateScheduleDuplicateTargetIDsIs400(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t1")

	rec := do(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "dup", "cron": "@hourly", "enabled": true, "timezone": "UTC",
		"target_ids": []int64{id, id}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Error.Message, "duplicates") {
		t.Fatalf("message = %q, want mention of duplicates", body.Error.Message)
	}
}

// TestReloadSchedulesSurvivesCancelledRequestContext ensures a client
// disconnect (cancelled request context) doesn't prevent the reload from
// running with a live context. Exercised directly against
// Deps.reloadSchedules since going through the full HTTP stack would also
// cancel the store reads validateSchedule needs before reload ever runs.
func TestReloadSchedulesSurvivesCancelledRequestContext(t *testing.T) {
	invoked := false
	d := Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		ReloadSchedules: func(ctx context.Context) error {
			invoked = true
			// Checked inside the callback: reloadSchedules defers its own
			// cancel(), so the context is only guaranteed live while
			// ReloadSchedules itself is running, same as any WithTimeout.
			if ctx.Err() != nil {
				t.Fatalf("reload ctx = %v, want a live context", ctx.Err())
			}
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate a client that already disconnected
	d.reloadSchedules(ctx)

	if !invoked {
		t.Fatal("ReloadSchedules was not invoked")
	}
}

func TestCreateScheduleDuplicateNameIs409(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t1")
	body := map[string]any{"name": "dup", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}}
	if rec := do(t, h, http.MethodPost, "/api/v1/schedules", body); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body.String())
	}
	rec := do(t, h, http.MethodPost, "/api/v1/schedules", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestListSchedulesIncludesNextRunOnlyWhenEnabled(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t1")
	do(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "on", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}})
	do(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "off", "cron": "@hourly", "enabled": false, "timezone": "UTC", "target_ids": []int64{id}})

	rec := do(t, h, http.MethodGet, "/api/v1/schedules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Schedules []struct {
			Name    string `json:"name"`
			NextRun string `json:"next_run"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Schedules) != 2 {
		t.Fatalf("schedules = %+v", body.Schedules)
	}
	for _, s := range body.Schedules {
		if s.Name == "on" && s.NextRun == "" {
			t.Fatal("enabled schedule has no next_run")
		}
		if s.Name == "off" && s.NextRun != "" {
			t.Fatalf("disabled schedule has next_run %q", s.NextRun)
		}
	}
}

func TestUpdateAndDeleteScheduleReload(t *testing.T) {
	h, _, _ := newTestAPI(t)
	reloads := 0
	withReload(func() { reloads++ })
	id := createTestTarget(t, h, "t1")
	rec := do(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "s", "cron": "@hourly", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}})
	var created struct {
		Schedule struct {
			ID int64 `json:"id"`
		} `json:"schedule"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	path := "/api/v1/schedules/" + itoa(created.Schedule.ID)
	if rec := do(t, h, http.MethodPut, path, map[string]any{
		"name": "s2", "cron": "*/15 * * * *", "enabled": false, "timezone": "UTC",
		"target_ids": []int64{id}}); rec.Code != http.StatusOK {
		t.Fatalf("put status %d body %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusOK {
		t.Fatalf("get status %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status %d", rec.Code)
	}
	if reloads != 3 { // create + update + delete
		t.Fatalf("reloads = %d, want 3", reloads)
	}
}

func TestValidateCronEndpoint(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodPost, "/api/v1/schedules/validate",
		map[string]any{"cron": "*/15 * * * *", "timezone": "UTC"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK   bool     `json:"ok"`
		Next []string `json:"next"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Next) != 5 {
		t.Fatalf("body = %+v", body)
	}

	bad := do(t, h, http.MethodPost, "/api/v1/schedules/validate",
		map[string]any{"cron": "nonsense", "timezone": "UTC"})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad cron status %d", bad.Code)
	}
}

func TestRunScheduleEnqueuesManualRunWithOrderedTargets(t *testing.T) {
	h, _, run := newTestAPI(t)
	a := createTestTarget(t, h, "a")
	b := createTestTarget(t, h, "b")
	rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "s", "cron": "@hourly", "enabled": true, "timezone": "UTC",
		"target_ids": []int64{b, a}})
	var created struct {
		Schedule struct {
			ID int64 `json:"id"`
		} `json:"schedule"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	got := doJSON(t, h, http.MethodPost, "/api/v1/schedules/"+itoa(created.Schedule.ID)+"/run", nil)
	if got.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s", got.Code, got.Body.String())
	}
	req := run.lastReq
	if req.Trigger != "manual" || req.ScheduleID == nil || *req.ScheduleID != created.Schedule.ID {
		t.Fatalf("request = %+v", req)
	}
	if len(req.TargetIDs) != 2 || req.TargetIDs[0] != b || req.TargetIDs[1] != a {
		t.Fatalf("target ids = %v, want %v", req.TargetIDs, []int64{b, a})
	}
}

func TestRunUnknownScheduleIs404(t *testing.T) {
	h, _, _ := newTestAPI(t)
	if rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules/4242/run", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestScheduleNextReturnsFiveTimes(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t")
	rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "s", "cron": "*/15 * * * *", "enabled": true, "timezone": "UTC", "target_ids": []int64{id}})
	var created struct {
		Schedule struct {
			ID int64 `json:"id"`
		} `json:"schedule"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	got := doJSON(t, h, http.MethodGet, "/api/v1/schedules/"+itoa(created.Schedule.ID)+"/next", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("status %d body %s", got.Code, got.Body.String())
	}
	var body struct {
		Next []string `json:"next"`
	}
	json.Unmarshal(got.Body.Bytes(), &body)
	if len(body.Next) != 5 {
		t.Fatalf("next = %v", body.Next)
	}
}

func TestSaveScheduleReturnsOverlapWarning(t *testing.T) {
	h, _, _ := newTestAPI(t)
	a := createTestTarget(t, h, "a")
	b := createTestTarget(t, h, "b")
	if rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "first", "cron": "0 * * * *", "enabled": true, "timezone": "UTC",
		"target_ids": []int64{a}}); rec.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "second", "cron": "0 * * * *", "enabled": true, "timezone": "UTC",
		"target_ids": []int64{b}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("second: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Warnings []string `json:"warnings"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Warnings) != 1 || !strings.Contains(body.Warnings[0], "first") {
		t.Fatalf("warnings = %v", body.Warnings)
	}
}

func TestSaveDisabledScheduleHasNoWarnings(t *testing.T) {
	h, _, _ := newTestAPI(t)
	a := createTestTarget(t, h, "a")
	b := createTestTarget(t, h, "b")
	doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "first", "cron": "0 * * * *", "enabled": true, "timezone": "UTC", "target_ids": []int64{a}})
	rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "second", "cron": "0 * * * *", "enabled": false, "timezone": "UTC", "target_ids": []int64{b}})
	var body struct {
		Warnings []string `json:"warnings"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", body.Warnings)
	}
}

func TestListRunsFilteredByScheduleID(t *testing.T) {
	h, db, _ := newTestAPI(t)
	ctx := context.Background()
	tid := createTestTarget(t, h, "t")
	sid, err := db.CreateSchedule(ctx, &store.Schedule{
		Name: "s", Cron: "@hourly", Enabled: true, Timezone: "UTC", TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRun(ctx, "manual", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRun(ctx, "cron", &sid); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, h, http.MethodGet, "/api/v1/runs?schedule_id="+itoa(sid), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Runs []store.Run `json:"runs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Runs) != 1 || body.Runs[0].ScheduleID == nil || *body.Runs[0].ScheduleID != sid {
		t.Fatalf("runs = %+v", body.Runs)
	}
}

// TestListRunsRejectsZeroScheduleID checks that ?schedule_id=0 (unset id,
// not a real schedule) is a 400 rather than silently matching no rows.
func TestListRunsRejectsZeroScheduleID(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := doJSON(t, h, http.MethodGet, "/api/v1/runs?schedule_id=0", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestScheduleNextReturnsEmptyForDisabledSchedule(t *testing.T) {
	h, _, _ := newTestAPI(t)
	id := createTestTarget(t, h, "t")
	rec := doJSON(t, h, http.MethodPost, "/api/v1/schedules", map[string]any{
		"name": "s", "cron": "*/15 * * * *", "enabled": false, "timezone": "UTC", "target_ids": []int64{id}})
	var created struct {
		Schedule struct {
			ID int64 `json:"id"`
		} `json:"schedule"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	got := doJSON(t, h, http.MethodGet, "/api/v1/schedules/"+itoa(created.Schedule.ID)+"/next", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("status %d body %s", got.Code, got.Body.String())
	}
	var body struct {
		Next []string `json:"next"`
	}
	json.Unmarshal(got.Body.Bytes(), &body)
	if len(body.Next) != 0 {
		t.Fatalf("next = %v, want empty", body.Next)
	}
}
