package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

type resultsPage struct {
	Results    []store.Result `json:"results"`
	NextCursor string         `json:"next_cursor"`
}

func seedResults(t *testing.T, db *store.Store, n int) (int64, []int64) {
	t.Helper()
	ctx := context.Background()
	tid, err := db.CreateTarget(ctx, &store.Target{
		Name: "home", Engine: "fake", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"download_bps":7}`)})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := 0; i < n; i++ {
		id, err := db.InsertResult(ctx, &store.Result{
			TargetID: &tid, TargetName: "home", Engine: "fake", Status: "ok",
			StartedAt:       "2026-09-13T1" + strconv.Itoa(i) + ":00:00.000Z",
			OptionsSnapshot: json.RawMessage(`{"download_bps":7}`),
			DownloadBps:     1e8,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return tid, ids
}

func TestListResultsCursorPagination(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 5)

	rec := do(t, h, http.MethodGet, "/api/v1/results?limit=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	var page resultsPage
	json.NewDecoder(rec.Body).Decode(&page)
	if len(page.Results) != 2 || page.Results[0].ID != ids[4] {
		t.Fatalf("page1 = %+v", page.Results)
	}
	if page.NextCursor == "" {
		t.Fatal("next_cursor empty on a full page")
	}

	rec = do(t, h, http.MethodGet, "/api/v1/results?limit=2&cursor="+page.NextCursor, nil)
	var page2 resultsPage
	json.NewDecoder(rec.Body).Decode(&page2)
	if len(page2.Results) != 2 || page2.Results[0].ID != ids[2] {
		t.Fatalf("page2 = %+v", page2.Results)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/results?cursor=not-a-number", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor = %d, want 400", rec.Code)
	}
}

func TestListResultsFilters(t *testing.T) {
	h, db, _ := newTestAPI(t)
	tid, _ := seedResults(t, db, 3)

	rec := do(t, h, http.MethodGet, "/api/v1/results?target_id="+itoa(tid)+"&engine=fake&status=ok", nil)
	var page resultsPage
	json.NewDecoder(rec.Body).Decode(&page)
	if len(page.Results) != 3 {
		t.Errorf("filtered = %d, want 3", len(page.Results))
	}

	rec = do(t, h, http.MethodGet,
		"/api/v1/results?from=2026-09-13T11:00:00.000Z&to=2026-09-13T11:59:59.000Z", nil)
	var ranged resultsPage
	json.NewDecoder(rec.Body).Decode(&ranged)
	if len(ranged.Results) != 1 {
		t.Errorf("range = %d, want 1", len(ranged.Results))
	}

	if rec := do(t, h, http.MethodGet, "/api/v1/results?target_id=abc", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad target_id = %d, want 400", rec.Code)
	}
}

func TestListResultsTagFilter(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 2)

	rec := do(t, h, http.MethodPut, "/api/v1/results/"+itoa(ids[0])+"/tags", map[string]any{"tags": []string{"night"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT tags = %d body=%s", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/results?tag=night", nil)
	var page resultsPage
	json.NewDecoder(rec.Body).Decode(&page)
	if len(page.Results) != 1 || page.Results[0].ID != ids[0] {
		t.Fatalf("tag filter = %+v", page.Results)
	}

	// Normalisation: mixed case/whitespace still matches.
	rec = do(t, h, http.MethodGet, "/api/v1/results?tag=%20NIGHT%20", nil)
	json.NewDecoder(rec.Body).Decode(&page)
	if len(page.Results) != 1 {
		t.Errorf("normalised tag filter = %+v", page.Results)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/results?tag=nonexistent", nil)
	var none resultsPage
	json.NewDecoder(rec.Body).Decode(&none)
	if len(none.Results) != 0 {
		t.Errorf("unknown tag = %+v", none.Results)
	}
}

func TestListResultsFromToValidation(t *testing.T) {
	h, db, _ := newTestAPI(t)
	seedResults(t, db, 3)

	// RFC3339 without fractional seconds and unix seconds both work.
	rec := do(t, h, http.MethodGet, "/api/v1/results?from=2026-09-13T00:00:00Z", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("rfc3339 from = %d body=%s", rec.Code, rec.Body)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/results?from=1789084800", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("unix seconds from = %d body=%s", rec.Code, rec.Body)
	}

	if rec := do(t, h, http.MethodGet, "/api/v1/results?from=not-a-time", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad from = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/results?to=also-not-a-time", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad to = %d, want 400", rec.Code)
	}
}

func TestGetDeleteAndTagResult(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 1)
	path := "/api/v1/results/" + itoa(ids[0])

	rec := do(t, h, http.MethodGet, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}

	rec = do(t, h, http.MethodPut, path+"/tags", map[string]any{"tags": []string{"Night", "night", "isp"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT tags = %d body=%s", rec.Code, rec.Body)
	}
	var tagged struct {
		Tags []string `json:"tags"`
	}
	json.NewDecoder(rec.Body).Decode(&tagged)
	if len(tagged.Tags) != 2 || tagged.Tags[0] != "isp" || tagged.Tags[1] != "night" {
		t.Errorf("tags = %v", tagged.Tags)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/tags", nil)
	var tags []store.Tag
	json.NewDecoder(rec.Body).Decode(&tags)
	if len(tags) != 2 {
		t.Errorf("GET tags = %+v", tags)
	}

	if rec := do(t, h, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET after delete = %d", rec.Code)
	}
}

func TestSetResultTagsValidation(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 1)
	path := "/api/v1/results/" + itoa(ids[0]) + "/tags"

	tooLong := ""
	for i := 0; i < 41; i++ {
		tooLong += "a"
	}
	if rec := do(t, h, http.MethodPut, path, map[string]any{"tags": []string{tooLong}}); rec.Code != http.StatusBadRequest {
		t.Errorf("tag too long = %d, want 400", rec.Code)
	}
	if rec := do(t, h, http.MethodPut, path, map[string]any{"tags": []string{"   "}}); rec.Code != http.StatusBadRequest {
		t.Errorf("blank tag = %d, want 400", rec.Code)
	}

	many := make([]string, 21)
	for i := range many {
		many[i] = "tag" + strconv.Itoa(i)
	}
	if rec := do(t, h, http.MethodPut, path, map[string]any{"tags": many}); rec.Code != http.StatusBadRequest {
		t.Errorf("21 tags = %d, want 400", rec.Code)
	}

	ok := many[:20]
	if rec := do(t, h, http.MethodPut, path, map[string]any{"tags": ok}); rec.Code != http.StatusOK {
		t.Errorf("20 tags = %d, want 200", rec.Code)
	}
}

func TestReexecuteResultEnqueuesTarget(t *testing.T) {
	h, db, run := newTestAPI(t)
	tid, ids := seedResults(t, db, 1)

	original, err := db.GetResult(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}

	rec := do(t, h, http.MethodPost, "/api/v1/results/"+itoa(ids[0])+"/reexecute", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
	if run.lastReq.Trigger != "reexec" {
		t.Errorf("trigger = %q, want reexec", run.lastReq.Trigger)
	}
	if len(run.lastReq.TargetIDs) != 1 || run.lastReq.TargetIDs[0] != tid {
		t.Errorf("targets = %v, want [%d]", run.lastReq.TargetIDs, tid)
	}
	if string(run.lastReq.Snapshots[tid]) != string(original.OptionsSnapshot) {
		t.Errorf("snapshot = %s, want %s", run.lastReq.Snapshots[tid], original.OptionsSnapshot)
	}

	// A result whose target has been deleted cannot be replayed.
	if err := db.DeleteTarget(context.Background(), tid); err != nil {
		t.Fatal(err)
	}
	rec = do(t, h, http.MethodPost, "/api/v1/results/"+itoa(ids[0])+"/reexecute", nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("orphan reexecute = %d, want 400", rec.Code)
	}
}

func TestRunsRoutes(t *testing.T) {
	h, db, run := newTestAPI(t)
	ctx := context.Background()
	tid, _ := db.CreateTarget(ctx, &store.Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})

	rec := do(t, h, http.MethodPost, "/api/v1/runs", map[string]any{"target_ids": []int64{tid}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /runs = %d body=%s", rec.Code, rec.Body)
	}
	var accepted struct {
		RunID int64 `json:"run_id"`
	}
	json.NewDecoder(rec.Body).Decode(&accepted)
	if accepted.RunID != 77 {
		t.Errorf("run_id = %d", accepted.RunID)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/runs", map[string]any{"target_ids": []int64{}}); rec.Code != http.StatusBadRequest {
		t.Errorf("empty target_ids = %d, want 400", rec.Code)
	}

	id, _ := db.CreateRun(ctx, "manual", nil)
	rec = do(t, h, http.MethodGet, "/api/v1/runs/"+itoa(id), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET run = %d", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/api/v1/runs?limit=10", nil)
	var runsPage struct {
		Runs       []store.Run `json:"runs"`
		NextCursor string      `json:"next_cursor"`
	}
	json.NewDecoder(rec.Body).Decode(&runsPage)
	if len(runsPage.Runs) != 1 {
		t.Errorf("runs = %+v", runsPage.Runs)
	}

	if rec := do(t, h, http.MethodDelete, "/api/v1/runs/"+itoa(id), nil); rec.Code != http.StatusAccepted {
		t.Errorf("DELETE run = %d", rec.Code)
	}
	if len(run.canceled) != 1 || run.canceled[0] != id {
		t.Errorf("cancel calls = %v", run.canceled)
	}
	run.cancelOK = false
	if rec := do(t, h, http.MethodDelete, "/api/v1/runs/"+itoa(id), nil); rec.Code != http.StatusConflict {
		t.Errorf("cancel of a finished run = %d, want 409", rec.Code)
	}
}
