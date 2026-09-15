package store

import (
	"context"
	"testing"
	"time"
)

func TestSessionCRUD(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	sess := Session{
		ID:        "hash-1",
		Subject:   "sub-1",
		Email:     "alice@example.com",
		Name:      "Alice",
		Groups:    []string{"admins", "users"},
		IsAdmin:   true,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	if err := db.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.LookupSession(ctx, "hash-1", now)
	if err != nil || !ok {
		t.Fatalf("LookupSession = %+v ok %v err %v", got, ok, err)
	}
	if got.Subject != "sub-1" || got.Email != "alice@example.com" || got.Name != "Alice" ||
		!got.IsAdmin || len(got.Groups) != 2 || got.Groups[0] != "admins" {
		t.Fatalf("got = %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("timestamps = %+v", got)
	}

	// expired
	if _, ok, err := db.LookupSession(ctx, "hash-1", now.Add(2*time.Hour)); err != nil || ok {
		t.Fatalf("expired lookup ok=%v err=%v, want ok=false", ok, err)
	}

	// unknown
	if _, ok, err := db.LookupSession(ctx, "nope", now); err != nil || ok {
		t.Fatalf("unknown lookup ok=%v err=%v, want ok=false", ok, err)
	}

	if err := db.DeleteSession(ctx, "hash-1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.LookupSession(ctx, "hash-1", now); ok {
		t.Fatal("session found after delete")
	}
	// deleting again is a no-op, not an error
	if err := db.DeleteSession(ctx, "hash-1"); err != nil {
		t.Fatalf("delete missing session = %v, want nil", err)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(id string, exp time.Time) {
		if err := db.CreateSession(ctx, Session{
			ID: id, Subject: "s", CreatedAt: now, ExpiresAt: exp,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("expired-1", now.Add(-time.Hour))
	mk("expired-2", now.Add(-time.Minute))
	mk("live-1", now.Add(time.Hour))

	n, err := db.DeleteExpiredSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	if _, ok, _ := db.LookupSession(ctx, "live-1", now); !ok {
		t.Fatal("live session removed")
	}
}
