package store

import (
	"context"
	"testing"
)

// insertTestTarget creates a minimal target and returns its id, for tests
// that only need a valid target_id to satisfy the notification_state
// foreign key.
func insertTestTarget(t *testing.T, s *Store, ctx context.Context) int64 {
	t.Helper()
	id, err := s.CreateTarget(ctx, &Target{Name: "t", Engine: "ookla", QueueID: 1})
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	return id
}

func TestNotifyStateUpsertAndGet(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id := insertTestTarget(t, s, ctx)

	if _, ok, err := s.GetNotifyState(ctx, id, "download"); err != nil || ok {
		t.Fatalf("GetNotifyState on empty table = ok %v err %v, want false nil", ok, err)
	}

	want := NotifyState{TargetID: id, Metric: "download",
		LastFiredAt: "2026-09-14T10:00:00.000Z", Firing: true}
	if err := s.SetNotifyState(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetNotifyState(ctx, id, "download")
	if err != nil || !ok || got != want {
		t.Fatalf("GetNotifyState = %+v ok %v err %v, want %+v", got, ok, err, want)
	}

	want.Firing = false
	want.LastFiredAt = "2026-09-14T11:00:00.000Z"
	if err := s.SetNotifyState(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, _, err = s.GetNotifyState(ctx, id, "download")
	if err != nil || got != want {
		t.Fatalf("after upsert = %+v err %v, want %+v", got, err, want)
	}
}

func TestNotifyStateClearAndCascade(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id := insertTestTarget(t, s, ctx)

	if err := s.SetNotifyState(ctx, NotifyState{TargetID: id, Metric: "ping", Firing: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotifyState(ctx, NotifyState{TargetID: id, Metric: "loss", Firing: true}); err != nil {
		t.Fatal(err)
	}

	if err := s.ClearNotifyState(ctx, id, "ping"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetNotifyState(ctx, id, "ping"); ok {
		t.Fatal("ping state still present after ClearNotifyState")
	}
	if _, ok, _ := s.GetNotifyState(ctx, id, "loss"); !ok {
		t.Fatal("ClearNotifyState removed the wrong metric")
	}

	// Deleting the target must cascade: a stale row would otherwise keep a
	// recreated target permanently "firing".
	if err := s.DeleteTarget(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetNotifyState(ctx, id, "loss"); ok {
		t.Fatal("notification_state row survived target deletion")
	}
}
