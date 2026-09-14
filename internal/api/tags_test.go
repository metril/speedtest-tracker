package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRenameAndDeleteTagEndpoints(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 1)
	if _, err := db.SetResultTags(t.Context(), ids[0], []string{"evening"}); err != nil {
		t.Fatal(err)
	}
	tags, _ := db.ListTags(t.Context())
	id := tags[0].ID

	rec := do(t, h, http.MethodPut, "/api/v1/tags/"+itoa(id), strings.NewReader(`{"name":"Morning"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d body=%s", rec.Code, rec.Body)
	}
	var got struct {
		Name string `json:"name"`
	}
	json.NewDecoder(rec.Body).Decode(&got)
	if got.Name != "morning" {
		t.Errorf("name = %q", got.Name)
	}

	if rec := do(t, h, http.MethodPut, "/api/v1/tags/999999", strings.NewReader(`{"name":"x"}`)); rec.Code != http.StatusNotFound {
		t.Errorf("rename missing tag status = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/tags/"+itoa(id), nil); rec.Code != http.StatusNoContent {
		t.Errorf("delete status = %d", rec.Code)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/tags/"+itoa(id), nil); rec.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d", rec.Code)
	}
}

func TestRenameTagConflictIs409(t *testing.T) {
	h, db, _ := newTestAPI(t)
	_, ids := seedResults(t, db, 1)
	if _, err := db.SetResultTags(t.Context(), ids[0], []string{"evening", "wifi"}); err != nil {
		t.Fatal(err)
	}
	tags, _ := db.ListTags(t.Context())
	rec := do(t, h, http.MethodPut, "/api/v1/tags/"+itoa(tags[0].ID), strings.NewReader(`{"name":"`+tags[1].Name+`"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body)
	}
}
