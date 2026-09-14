package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

func TestTargetRevisionsListAndChangedFields(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": map[string]any{},
	})
	var created store.Target
	json.NewDecoder(rec.Body).Decode(&created)
	path := "/api/v1/targets/" + itoa(created.ID)

	do(t, h, http.MethodPut, path, map[string]any{
		"name": "renamed", "engine": "fake", "enabled": true, "lane": "wan",
		"options": map[string]any{},
	})
	do(t, h, http.MethodPut, path, map[string]any{
		"name": "renamed", "engine": "fake", "enabled": false, "lane": "lan",
		"options": map[string]any{},
	})

	rec = do(t, h, http.MethodGet, path+"/revisions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET revisions = %d body=%s", rec.Code, rec.Body)
	}
	var revs []struct {
		Version   int             `json:"version"`
		Action    string          `json:"action"`
		CreatedAt string          `json:"created_at"`
		Snapshot  json.RawMessage `json:"snapshot"`
		Changed   []string        `json:"changed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&revs); err != nil {
		t.Fatal(err)
	}
	if len(revs) != 3 {
		t.Fatalf("revs = %d, want 3", len(revs))
	}
	// Newest first.
	if revs[0].Version != 3 || revs[0].Action != "update" {
		t.Errorf("revs[0] = %+v", revs[0])
	}
	wantChanged := map[string]bool{"enabled": true, "lane": true}
	if len(revs[0].Changed) != 2 {
		t.Errorf("revs[0].Changed = %v, want %v", revs[0].Changed, wantChanged)
	}
	for _, f := range revs[0].Changed {
		if !wantChanged[f] {
			t.Errorf("unexpected changed field %q", f)
		}
	}
	if revs[1].Version != 2 || len(revs[1].Changed) != 1 || revs[1].Changed[0] != "name" {
		t.Errorf("revs[1] = %+v", revs[1])
	}
	if revs[2].Version != 1 || revs[2].Action != "create" || len(revs[2].Changed) != 0 {
		t.Errorf("revs[2] = %+v, want empty changed", revs[2])
	}
}

func TestRevertTargetRevision(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": map[string]any{},
	})
	var created store.Target
	json.NewDecoder(rec.Body).Decode(&created)
	path := "/api/v1/targets/" + itoa(created.ID)

	do(t, h, http.MethodPut, path, map[string]any{
		"name": "renamed", "engine": "fake", "enabled": false, "lane": "lan",
		"options": map[string]any{},
	})

	rec = do(t, h, http.MethodPost, path+"/revisions/1/revert", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revert = %d body=%s", rec.Code, rec.Body)
	}
	var reverted store.Target
	json.NewDecoder(rec.Body).Decode(&reverted)
	if reverted.Name != "home" || !reverted.Enabled || reverted.Lane != "wan" {
		t.Errorf("reverted = %+v", reverted)
	}

	rec = do(t, h, http.MethodGet, path+"/revisions", nil)
	var revs []struct {
		Version int    `json:"version"`
		Action  string `json:"action"`
	}
	json.NewDecoder(rec.Body).Decode(&revs)
	if len(revs) != 3 || revs[0].Action != "revert" || revs[0].Version != 3 {
		t.Fatalf("revisions after revert = %+v", revs)
	}

	if rec := do(t, h, http.MethodPost, path+"/revisions/99/revert", nil); rec.Code != http.StatusNotFound {
		t.Errorf("revert missing version = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/targets/999999/revisions/1/revert", nil); rec.Code != http.StatusNotFound {
		t.Errorf("revert missing target = %d, want 404", rec.Code)
	}
}

func TestDeletedTargetsListAndRestore(t *testing.T) {
	h, _, _ := newTestAPI(t)

	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "lane": "wan",
		"options": map[string]any{},
	})
	var created store.Target
	json.NewDecoder(rec.Body).Decode(&created)
	path := "/api/v1/targets/" + itoa(created.ID)

	if rec := do(t, h, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/targets/deleted", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET deleted = %d body=%s", rec.Code, rec.Body)
	}
	var deleted []store.DeletedTarget
	json.NewDecoder(rec.Body).Decode(&deleted)
	if len(deleted) != 1 || deleted[0].ID != created.ID || deleted[0].Name != "home" {
		t.Fatalf("deleted = %+v", deleted)
	}

	rec = do(t, h, http.MethodPost, "/api/v1/targets/deleted/"+itoa(created.ID)+"/restore", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore = %d body=%s", rec.Code, rec.Body)
	}
	var restored store.Target
	json.NewDecoder(rec.Body).Decode(&restored)
	if restored.ID != created.ID || restored.Name != "home" {
		t.Errorf("restored = %+v", restored)
	}

	rec = do(t, h, http.MethodGet, "/api/v1/targets/deleted", nil)
	var deleted2 []store.DeletedTarget
	json.NewDecoder(rec.Body).Decode(&deleted2)
	if len(deleted2) != 0 {
		t.Errorf("deleted after restore = %+v, want empty", deleted2)
	}

	// Once restored, the target is live again: it no longer shows up as
	// deleted, so a repeat restore call reports 404, not 409. (409 is
	// RestoreTarget's own race guard, exercised directly at the store
	// layer in TestRestoreTargetConflictsWithLiveRow.)
	if rec := do(t, h, http.MethodPost, "/api/v1/targets/deleted/"+itoa(created.ID)+"/restore", nil); rec.Code != http.StatusNotFound {
		t.Errorf("restore already-live target = %d, want 404", rec.Code)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/targets/deleted/999999/restore", nil); rec.Code != http.StatusNotFound {
		t.Errorf("restore never-deleted id = %d, want 404", rec.Code)
	}
}

// TestTargetsDeletedRouteNotShadowedByIDParam guards chi's static-over-param
// precedence: /targets/deleted must never be captured by the /targets/{id}
// route (which would 400 on "deleted" not being an integer).
func TestTargetsDeletedRouteNotShadowedByIDParam(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodGet, "/api/v1/targets/deleted", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /targets/deleted = %d body=%s, want 200 (not shadowed by /targets/{id})", rec.Code, rec.Body)
	}
}
