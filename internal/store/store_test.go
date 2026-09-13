package store

import (
	"context"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTemp(t)
	var n int
	err := s.Read.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN
		 ('targets','schedules','schedule_targets','runs','results','tags',
		  'result_tags','settings','notification_state','schema_migrations')`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 10 {
		t.Errorf("tables = %d, want 10", n)
	}
}

func TestTargetsHasLaneColumnDefaultWan(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Write.Exec(`INSERT INTO targets(name,engine) VALUES('t','ookla')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var lane string
	if err := s.Read.QueryRow(`SELECT lane FROM targets WHERE name='t'`).Scan(&lane); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if lane != "wan" {
		t.Errorf("lane = %q, want wan", lane)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	s1.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()
	var n int
	if err := s2.Read.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", n)
	}
}

func TestPragmas(t *testing.T) {
	s := openTemp(t)
	for _, tc := range []struct{ pragma, want string }{
		{"journal_mode", "wal"},
		{"foreign_keys", "1"},
		{"busy_timeout", "5000"},
		{"synchronous", "1"},
	} {
		var got string
		if err := s.Write.QueryRow("PRAGMA " + tc.pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", tc.pragma, err)
		}
		if got != tc.want {
			t.Errorf("PRAGMA %s = %q, want %q", tc.pragma, got, tc.want)
		}
	}
}

func TestForeignKeysCascade(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Write.Exec(`INSERT INTO targets(id,name,engine) VALUES(1,'t','ookla')`); err != nil {
		t.Fatalf("target: %v", err)
	}
	if _, err := s.Write.Exec(
		`INSERT INTO notification_state(target_id,metric) VALUES(1,'download')`); err != nil {
		t.Fatalf("state: %v", err)
	}
	if _, err := s.Write.Exec(`DELETE FROM targets WHERE id=1`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	if err := s.Read.QueryRow(`SELECT count(*) FROM notification_state`).Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 0 {
		t.Errorf("notification_state rows = %d, want 0 (cascade)", n)
	}
}

func TestPingAndWritePoolIsSingleConn(t *testing.T) {
	s := openTemp(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got := s.Write.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write MaxOpenConnections = %d, want 1", got)
	}
}
