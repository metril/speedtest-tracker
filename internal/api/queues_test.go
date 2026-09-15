package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/metril/speedtest-tracker/internal/store"
)

func TestQueuesCRUDRoutes(t *testing.T) {
	h, _, _ := newTestAPI(t)

	var created store.Queue
	rec := doJSON(t, h, http.MethodPost, "/api/v1/queues", map[string]any{"name": "office"}, &created)
	if rec.Code != http.StatusCreated || created.Name != "office" {
		t.Fatalf("POST = %d %+v", rec.Code, created)
	}

	var list []store.Queue
	rec = doJSON(t, h, http.MethodGet, "/api/v1/queues", nil, &list)
	if rec.Code != http.StatusOK || len(list) != 3 {
		t.Fatalf("GET list = %d %+v", rec.Code, list)
	}

	var renamed store.Queue
	path := "/api/v1/queues/" + itoa(created.ID)
	rec = doJSON(t, h, http.MethodPut, path, map[string]any{"name": "branch"}, &renamed)
	if rec.Code != http.StatusOK || renamed.Name != "branch" {
		t.Fatalf("PUT = %d %+v", rec.Code, renamed)
	}

	rec = do(t, h, http.MethodDelete, path, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
}

func TestCreateQueueRejectsInvalidName(t *testing.T) {
	h, _, _ := newTestAPI(t)
	for _, name := range []string{"", "   ", string(make([]byte, 65))} {
		rec := do(t, h, http.MethodPost, "/api/v1/queues", map[string]any{"name": name})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q = %d, want 400", name, rec.Code)
		}
	}
}

func TestCreateQueueNameConflict(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodPost, "/api/v1/queues", map[string]any{"name": "wan"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("POST duplicate = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "name_conflict" {
		t.Errorf("code = %q, want name_conflict", body.Error.Code)
	}
}

func TestDeleteQueueInUseReturns409(t *testing.T) {
	h, db, _ := newTestAPI(t)
	wan, err := db.GetQueueByName(t.Context(), "wan")
	if err != nil {
		t.Fatalf("GetQueueByName: %v", err)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "queue_id": wan.ID,
		"options": map[string]any{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create target = %d body=%s", rec.Code, rec.Body)
	}

	rec = do(t, h, http.MethodDelete, "/api/v1/queues/"+itoa(wan.ID), nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("DELETE in-use queue = %d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.Code != "queue_in_use" {
		t.Errorf("code = %q, want queue_in_use", body.Error.Code)
	}
}

func TestDeleteLastQueueReturns409(t *testing.T) {
	h, db, _ := newTestAPI(t)
	queues, err := db.ListQueues(t.Context())
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	for _, q := range queues[:len(queues)-1] {
		if rec := do(t, h, http.MethodDelete, "/api/v1/queues/"+itoa(q.ID), nil); rec.Code != http.StatusNoContent {
			t.Fatalf("delete %d = %d", q.ID, rec.Code)
		}
	}
	last := queues[len(queues)-1]
	rec := do(t, h, http.MethodDelete, "/api/v1/queues/"+itoa(last.ID), nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("DELETE last queue = %d body=%s", rec.Code, rec.Body)
	}
}

// TestCreateTargetDefaultsToWanQueue covers the API back-compat contract:
// a create/update body that omits queue_id gets the wan queue, so existing
// clients that never heard of queues keep working.
func TestCreateTargetDefaultsToWanQueue(t *testing.T) {
	h, db, _ := newTestAPI(t)
	var created store.Target
	rec := doJSON(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "options": map[string]any{},
	}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST = %d body=%s", rec.Code, rec.Body)
	}
	wan, err := db.GetQueueByName(t.Context(), "wan")
	if err != nil {
		t.Fatalf("GetQueueByName: %v", err)
	}
	if created.QueueID != wan.ID || created.QueueName != "wan" {
		t.Errorf("created queue = %d/%s, want %d/wan", created.QueueID, created.QueueName, wan.ID)
	}
}

func TestCreateTargetRejectsUnknownQueueID(t *testing.T) {
	h, _, _ := newTestAPI(t)
	rec := do(t, h, http.MethodPost, "/api/v1/targets", map[string]any{
		"name": "home", "engine": "fake", "enabled": true, "queue_id": 99999,
		"options": map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST unknown queue_id = %d body=%s", rec.Code, rec.Body)
	}
}
