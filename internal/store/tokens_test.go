package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return openTemp(t)
}

func TestAPITokenCRUD(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	if n, err := db.CountAPITokens(ctx); err != nil || n != 0 {
		t.Fatalf("CountAPITokens on a fresh db = %d, %v", n, err)
	}
	tok, err := db.CreateAPIToken(ctx, "home assistant", "hash-1", "stt_abcd")
	if err != nil {
		t.Fatal(err)
	}
	if tok.ID == 0 || tok.Name != "home assistant" || tok.Prefix != "stt_abcd" || tok.CreatedAt == "" {
		t.Fatalf("created = %+v", tok)
	}
	if tok.LastUsedAt != "" {
		t.Fatalf("last_used_at = %q, want empty on a new token", tok.LastUsedAt)
	}

	got, ok, err := db.APITokenByHash(ctx, "hash-1")
	if err != nil || !ok || got.ID != tok.ID {
		t.Fatalf("APITokenByHash = %+v ok %v err %v", got, ok, err)
	}
	if _, ok, _ := db.APITokenByHash(ctx, "nope"); ok {
		t.Fatal("unknown hash resolved to a token")
	}

	if err := db.TouchAPIToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListAPITokens(ctx)
	if err != nil || len(list) != 1 || list[0].LastUsedAt == "" {
		t.Fatalf("ListAPITokens = %+v err %v, want one row with last_used_at set", list, err)
	}

	if err := db.DeleteAPIToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAPIToken(ctx, tok.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
	if n, _ := db.CountAPITokens(ctx); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestAPITokenHashIsUnique(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	db.CreateAPIToken(ctx, "a", "hash-1", "stt_a")
	if _, err := db.CreateAPIToken(ctx, "b", "hash-1", "stt_b"); err == nil {
		t.Fatal("duplicate hash accepted; the table must reject it")
	}
}

func TestListAPITokensNeverReturnsHashes(t *testing.T) {
	db := newTestStore(t)
	db.CreateAPIToken(context.Background(), "a", "hash-1", "stt_a")
	list, _ := db.ListAPITokens(context.Background())
	blob, _ := json.Marshal(list)
	if strings.Contains(string(blob), "hash-1") {
		t.Fatalf("APIToken JSON leaks the hash: %s", blob)
	}
}
