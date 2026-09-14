package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCreateTargetWritesCreateRevision(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	id, err := s.CreateTarget(ctx, &Target{
		Name: "home", Engine: "ookla", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"server_id":1234}`),
	})
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	revs, err := s.ListTargetRevisions(ctx, id)
	if err != nil {
		t.Fatalf("ListTargetRevisions: %v", err)
	}
	if len(revs) != 1 {
		t.Fatalf("revs = %d, want 1", len(revs))
	}
	if revs[0].Version != 1 || revs[0].Action != "create" || revs[0].TargetID != id {
		t.Errorf("rev = %+v", revs[0])
	}
	var snap Target
	if err := json.Unmarshal(revs[0].Snapshot, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snap.Name != "home" || snap.ID != id {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestUpdateTargetWritesUpdateRevisions(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, err := s.CreateTarget(ctx, &Target{Name: "home", Engine: "ookla", Enabled: true, Lane: "wan"})
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	got, _ := s.GetTarget(ctx, id)
	got.Name = "renamed"
	if err := s.UpdateTarget(ctx, got); err != nil {
		t.Fatalf("UpdateTarget: %v", err)
	}
	got.Lane = "lan"
	if err := s.UpdateTarget(ctx, got); err != nil {
		t.Fatalf("UpdateTarget 2: %v", err)
	}

	revs, err := s.ListTargetRevisions(ctx, id)
	if err != nil {
		t.Fatalf("ListTargetRevisions: %v", err)
	}
	if len(revs) != 3 {
		t.Fatalf("revs = %d, want 3", len(revs))
	}
	// Newest first.
	if revs[0].Version != 3 || revs[0].Action != "update" {
		t.Errorf("rev[0] = %+v", revs[0])
	}
	if revs[1].Version != 2 || revs[1].Action != "update" {
		t.Errorf("rev[1] = %+v", revs[1])
	}
	if revs[2].Version != 1 || revs[2].Action != "create" {
		t.Errorf("rev[2] = %+v", revs[2])
	}
}

func TestUpdateTargetMissingWritesNoRevision(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	err := s.UpdateTarget(ctx, &Target{ID: 999, Name: "x", Engine: "fake", Lane: "wan"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteTargetWritesDeleteRevisionWithPreDeleteSnapshot(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "ookla", Enabled: true, Lane: "wan"})

	if err := s.DeleteTarget(ctx, id); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}

	revs, err := s.ListTargetRevisions(ctx, id)
	if err != nil {
		t.Fatalf("ListTargetRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("revs = %d, want 2", len(revs))
	}
	if revs[0].Action != "delete" || revs[0].Version != 2 {
		t.Errorf("rev[0] = %+v", revs[0])
	}
	var snap Target
	json.Unmarshal(revs[0].Snapshot, &snap)
	if snap.Name != "home" || snap.ID != id {
		t.Errorf("delete snapshot = %+v", snap)
	}
}

func TestGetTargetRevision(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "ookla", Enabled: true, Lane: "wan"})

	rev, err := s.GetTargetRevision(ctx, id, 1)
	if err != nil {
		t.Fatalf("GetTargetRevision: %v", err)
	}
	if rev.Version != 1 || rev.Action != "create" {
		t.Errorf("rev = %+v", rev)
	}

	if _, err := s.GetTargetRevision(ctx, id, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing version err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetTargetRevision(ctx, 12345, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing target err = %v, want ErrNotFound", err)
	}
}

func TestRevisionPruneKeepsLast50(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})

	got, _ := s.GetTarget(ctx, id)
	for i := 0; i < 60; i++ {
		got.Lane = "lan"
		if got.Lane == "lan" {
			got.Lane = "wan"
		} else {
			got.Lane = "lan"
		}
		if err := s.UpdateTarget(ctx, got); err != nil {
			t.Fatalf("UpdateTarget %d: %v", i, err)
		}
	}
	// 1 create + 60 updates = 61 versions total, pruned to the newest 50.
	revs, err := s.ListTargetRevisions(ctx, id)
	if err != nil {
		t.Fatalf("ListTargetRevisions: %v", err)
	}
	if len(revs) != 50 {
		t.Fatalf("revs = %d, want 50", len(revs))
	}
	if revs[0].Version != 61 {
		t.Errorf("newest version = %d, want 61", revs[0].Version)
	}
	if revs[len(revs)-1].Version != 12 {
		t.Errorf("oldest kept version = %d, want 12", revs[len(revs)-1].Version)
	}
}

func TestListDeletedTargetsAndRestore(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, _ := s.CreateTarget(ctx, &Target{
		Name: "home", Engine: "ookla", Enabled: true, Lane: "wan",
		Options: json.RawMessage(`{"server_id":1234}`),
	})
	original, err := s.GetTarget(ctx, id)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if err := s.DeleteTarget(ctx, id); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}

	deleted, err := s.ListDeletedTargets(ctx)
	if err != nil {
		t.Fatalf("ListDeletedTargets: %v", err)
	}
	if len(deleted) != 1 || deleted[0].ID != id || deleted[0].Name != "home" {
		t.Fatalf("deleted = %+v", deleted)
	}

	snap, err := s.LatestDeletedSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("LatestDeletedSnapshot: %v", err)
	}
	if snap.ID != id || snap.Name != "home" {
		t.Fatalf("snapshot = %+v", snap)
	}

	restored, err := s.RestoreTarget(ctx, snap)
	if err != nil {
		t.Fatalf("RestoreTarget: %v", err)
	}
	if restored.ID != id || restored.Name != "home" || string(restored.Options) != `{"server_id":1234}` {
		t.Errorf("restored = %+v", restored)
	}
	if restored.CreatedAt != original.CreatedAt {
		t.Errorf("restored.CreatedAt = %q, want original %q", restored.CreatedAt, original.CreatedAt)
	}

	deleted2, err := s.ListDeletedTargets(ctx)
	if err != nil {
		t.Fatalf("ListDeletedTargets after restore: %v", err)
	}
	if len(deleted2) != 0 {
		t.Errorf("deleted after restore = %+v, want empty", deleted2)
	}

	revs, _ := s.ListTargetRevisions(ctx, id)
	if revs[0].Action != "restore" {
		t.Errorf("latest revision action = %q, want restore", revs[0].Action)
	}
}

func TestRestoreTargetConflictsWithLiveRow(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, _ := s.CreateTarget(ctx, &Target{Name: "home", Engine: "fake", Enabled: true, Lane: "wan"})
	s.DeleteTarget(ctx, id)
	snap, err := s.LatestDeletedSnapshot(ctx, id)
	if err != nil {
		t.Fatalf("LatestDeletedSnapshot: %v", err)
	}
	// First restore succeeds; calling RestoreTarget again with the same
	// id must fail because a live row now exists.
	if _, err := s.RestoreTarget(ctx, snap); err != nil {
		t.Fatalf("first RestoreTarget: %v", err)
	}
	if _, err := s.RestoreTarget(ctx, snap); !errors.Is(err, ErrIDConflict) {
		t.Errorf("second RestoreTarget err = %v, want ErrIDConflict", err)
	}
}
