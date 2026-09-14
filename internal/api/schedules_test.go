package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

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
