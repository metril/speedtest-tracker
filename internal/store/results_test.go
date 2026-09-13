package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// insertResultAt inserts a result with an explicit started_at so ordering
// and range filters are deterministic.
func insertResultAt(t *testing.T, s *Store, targetID int64, engine, status, startedAt string) int64 {
	t.Helper()
	id, err := s.InsertResult(context.Background(), &Result{
		TargetID: &targetID, TargetName: "t", Engine: engine, Status: status,
		StartedAt: startedAt, DurationMs: 1200,
		OptionsSnapshot: json.RawMessage(`{"server_id":1}`),
		DownloadBps:     100e6, UploadBps: 50e6, PingMs: 12.5,
		Raw: json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("InsertResult: %v", err)
	}
	return id
}

func TestInsertAndGetResult(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "ookla", Enabled: true, Lane: "wan"})
	id := insertResultAt(t, s, tid, "ookla", "ok", "2026-09-13T10:00:00.000Z")

	got, err := s.GetResult(ctx, id)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if got.Engine != "ookla" || got.Status != "ok" || got.DownloadBps != 100e6 || got.PingMs != 12.5 {
		t.Errorf("got %+v", got)
	}
	if string(got.OptionsSnapshot) != `{"server_id":1}` || string(got.Raw) != `{"ok":true}` {
		t.Errorf("json round trip: %s %s", got.OptionsSnapshot, got.Raw)
	}
	if got.TargetID == nil || *got.TargetID != tid {
		t.Errorf("target id = %v", got.TargetID)
	}

	if err := s.DeleteResult(ctx, id); err != nil {
		t.Fatalf("DeleteResult: %v", err)
	}
	if _, err := s.GetResult(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteResult(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice = %v, want ErrNotFound", err)
	}
}

func TestInsertResultFailedKeepsError(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, err := s.InsertResult(ctx, &Result{
		TargetName: "gone", Engine: "fake", Status: "failed",
		StartedAt: "2026-09-13T10:00:00.000Z", Error: "dial tcp: refused",
	})
	if err != nil {
		t.Fatalf("InsertResult: %v", err)
	}
	got, _ := s.GetResult(ctx, id)
	if got.Status != "failed" || got.Error != "dial tcp: refused" {
		t.Errorf("got %+v", got)
	}
	if string(got.OptionsSnapshot) != "{}" {
		t.Errorf("empty snapshot = %s, want {}", got.OptionsSnapshot)
	}
}

func TestListResultsFiltersAndKeyset(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "ookla", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, Lane: "lan"})

	var ids []int64
	for i := 0; i < 4; i++ {
		ids = append(ids, insertResultAt(t, s, a, "ookla", "ok",
			fmt.Sprintf("2026-09-13T1%d:00:00.000Z", i)))
	}
	bad := insertResultAt(t, s, b, "fake", "failed", "2026-09-13T20:00:00.000Z")

	all, next, err := s.ListResults(ctx, ResultFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListResults: %v", err)
	}
	if len(all) != 2 || all[0].ID != bad || all[1].ID != ids[3] {
		t.Fatalf("page1 = %+v", all)
	}
	if next != ids[3] {
		t.Fatalf("next = %d want %d", next, ids[3])
	}
	page2, _, err := s.ListResults(ctx, ResultFilter{Limit: 2, Cursor: next})
	if err != nil || len(page2) != 2 || page2[0].ID != ids[2] {
		t.Fatalf("page2 = %+v %v", page2, err)
	}

	byTarget, _, _ := s.ListResults(ctx, ResultFilter{TargetID: &a})
	if len(byTarget) != 4 {
		t.Errorf("target filter = %d, want 4", len(byTarget))
	}
	byEngine, _, _ := s.ListResults(ctx, ResultFilter{Engine: "fake"})
	if len(byEngine) != 1 || byEngine[0].ID != bad {
		t.Errorf("engine filter = %+v", byEngine)
	}
	byStatus, _, _ := s.ListResults(ctx, ResultFilter{Status: "failed"})
	if len(byStatus) != 1 || byStatus[0].ID != bad {
		t.Errorf("status filter = %+v", byStatus)
	}
	byRange, _, _ := s.ListResults(ctx, ResultFilter{
		From: "2026-09-13T11:00:00.000Z", To: "2026-09-13T13:00:00.000Z"})
	if len(byRange) != 3 {
		t.Errorf("range filter = %d, want 3", len(byRange))
	}
}

func TestLatestResults(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "ookla", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, Lane: "lan"})
	insertResultAt(t, s, a, "ookla", "ok", "2026-09-13T10:00:00.000Z")
	newestA := insertResultAt(t, s, a, "ookla", "ok", "2026-09-13T11:00:00.000Z")
	newestB := insertResultAt(t, s, b, "fake", "ok", "2026-09-13T09:00:00.000Z")

	got, err := s.LatestResultForTarget(ctx, a)
	if err != nil || got.ID != newestA {
		t.Fatalf("latest for a = %+v %v", got, err)
	}
	if _, err := s.LatestResultForTarget(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("latest for unknown = %v, want ErrNotFound", err)
	}

	all, err := s.LatestResults(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("LatestResults = %+v %v", all, err)
	}
	seen := map[int64]bool{all[0].ID: true, all[1].ID: true}
	if !seen[newestA] || !seen[newestB] {
		t.Errorf("LatestResults = %+v, want %d and %d", all, newestA, newestB)
	}
}
