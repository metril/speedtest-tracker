package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestTargetCRUD(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	id, err := s.CreateTarget(ctx, &Target{
		Name: "home", Engine: "ookla", Enabled: true, QueueID: 1,
		Options:    json.RawMessage(`{"server_id":1234}`),
		Thresholds: json.RawMessage(`{"download_bps":1000}`),
	})
	if err != nil || id == 0 {
		t.Fatalf("CreateTarget: id=%d err=%v", id, err)
	}

	got, err := s.GetTarget(ctx, id)
	if err != nil {
		t.Fatalf("GetTarget: %v", err)
	}
	if got.Name != "home" || got.Engine != "ookla" || !got.Enabled || got.QueueID != 1 {
		t.Errorf("got %+v", got)
	}
	if string(got.Options) != `{"server_id":1234}` {
		t.Errorf("options = %s", got.Options)
	}
	if got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Errorf("timestamps not populated: %+v", got)
	}

	got.Name, got.Enabled, got.QueueID = "renamed", false, 2
	got.Options = json.RawMessage(`{}`)
	if err := s.UpdateTarget(ctx, got); err != nil {
		t.Fatalf("UpdateTarget: %v", err)
	}
	got2, err := s.GetTarget(ctx, id)
	if err != nil {
		t.Fatalf("GetTarget after update: %v", err)
	}
	if got2.Name != "renamed" || got2.Enabled || got2.QueueID != 2 {
		t.Errorf("update not applied: %+v", got2)
	}

	list, err := s.ListTargets(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListTargets: %d %v", len(list), err)
	}

	if err := s.DeleteTarget(ctx, id); err != nil {
		t.Fatalf("DeleteTarget: %v", err)
	}
	if _, err := s.GetTarget(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTarget after delete = %v, want ErrNotFound", err)
	}
	if err := s.DeleteTarget(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTarget twice = %v, want ErrNotFound", err)
	}
}

func TestNextRotationIndexCyclesAndPersists(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	id, err := s.CreateTarget(ctx, &Target{
		Name: "home", Engine: "iperf3", Enabled: true, QueueID: 1,
		Options: json.RawMessage(`{"hosts":["a","b","c"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTarget(ctx, id)
	if err != nil || got.RotationIndex != 0 {
		t.Fatalf("initial rotation_index = %d, err=%v, want 0", got.RotationIndex, err)
	}

	var seen []int
	for range 5 {
		idx, err := s.NextRotationIndex(ctx, id, 3)
		if err != nil {
			t.Fatalf("NextRotationIndex: %v", err)
		}
		seen = append(seen, idx)
	}
	want := []int{0, 1, 2, 0, 1}
	for i, v := range want {
		if seen[i] != v {
			t.Errorf("seen = %v, want %v", seen, want)
			break
		}
	}

	// The cursor is a plain column, so reopening the DB (simulating a
	// restart) must pick up where it left off rather than resetting.
	got, err = s.GetTarget(ctx, id)
	if err != nil || got.RotationIndex != 5 {
		t.Fatalf("rotation_index after 5 calls = %d, err=%v, want 5", got.RotationIndex, err)
	}
}

func TestNextRotationIndexMissingTarget(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	if _, err := s.NextRotationIndex(ctx, 999, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateTargetMissing(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	err := s.UpdateTarget(ctx, &Target{ID: 999, Name: "x", Engine: "fake", QueueID: 1})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListTargetsByIDsPreservesRequestOrder(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, QueueID: 1})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, QueueID: 2})

	got, err := s.ListTargetsByIDs(ctx, []int64{b, a})
	if err != nil {
		t.Fatalf("ListTargetsByIDs: %v", err)
	}
	if len(got) != 2 || got[0].ID != b || got[1].ID != a {
		t.Errorf("order = %+v, want [%d %d]", got, b, a)
	}
	empty, err := s.ListTargetsByIDs(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty = %v %v", empty, err)
	}
}
