package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
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
		  'result_tags','settings','notification_state','schema_migrations','queues')`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 11 {
		t.Errorf("tables = %d, want 11", n)
	}
}

func TestTargetsQueueIDDefaultsToWan(t *testing.T) {
	s := openTemp(t)
	if _, err := s.Write.Exec(`INSERT INTO targets(name,engine) VALUES('t','ookla')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var queueName string
	if err := s.Read.QueryRow(
		`SELECT q.name FROM targets t JOIN queues q ON q.id=t.queue_id WHERE t.name='t'`,
	).Scan(&queueName); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if queueName != "wan" {
		t.Errorf("queue = %q, want wan", queueName)
	}
}

func TestQueuesSeededWanAndLan(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	queues, err := s.ListQueues(ctx)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	if len(queues) != 2 || queues[0].Name != "wan" || queues[1].Name != "lan" {
		t.Errorf("queues = %+v, want [wan lan]", queues)
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
	if n != 9 {
		t.Errorf("schema_migrations rows = %d, want 9", n)
	}
}

func TestPragmas(t *testing.T) {
	s := openTemp(t)
	for _, pool := range []struct {
		name string
		db   *sql.DB
	}{
		{"write", s.Write},
		{"read", s.Read},
	} {
		for _, tc := range []struct{ pragma, want string }{
			{"journal_mode", "wal"},
			{"foreign_keys", "1"},
			{"busy_timeout", "5000"},
			{"synchronous", "1"},
		} {
			var got string
			if err := pool.db.QueryRow("PRAGMA " + tc.pragma).Scan(&got); err != nil {
				t.Fatalf("%s pool PRAGMA %s: %v", pool.name, tc.pragma, err)
			}
			if got != tc.want {
				t.Errorf("%s pool PRAGMA %s = %q, want %q", pool.name, tc.pragma, got, tc.want)
			}
		}
	}
}

// TestReadPoolDoesNotTakeWriteLock ensures only the write pool uses
// _txlock=immediate. If the read pool inherited it, a read BeginTx would
// itself try to grab the write lock and block behind an open write
// transaction, defeating WAL's concurrent readers.
func TestReadPoolDoesNotTakeWriteLock(t *testing.T) {
	s := openTemp(t)

	wtx, err := s.Write.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("write BeginTx: %v", err)
	}
	defer wtx.Rollback()
	if _, err := wtx.Exec(`INSERT INTO targets(name,engine) VALUES('t','ookla')`); err != nil {
		t.Fatalf("write insert: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	rtx, err := s.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("read BeginTx blocked on write lock: %v", err)
	}
	defer rtx.Rollback()

	var n int
	if err := rtx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master`).Scan(&n); err != nil {
		t.Fatalf("read query blocked: %v", err)
	}
}

func TestOpenEscapesSpecialPathCharacters(t *testing.T) {
	dir, err := os.MkdirTemp("", "a?b#c")
	if err != nil {
		t.Skipf("cannot create dir with special chars on this filesystem: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	var n int
	if err := s.Read.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 9 {
		t.Errorf("schema_migrations rows = %d, want 9", n)
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

// TestMigration0002CreatesResultIndexes confirms migration 2 applies and
// creates the composite (col, id DESC) indexes ListResults relies on.
func TestMigration0002CreatesResultIndexes(t *testing.T) {
	s := openTemp(t)
	for _, name := range []string{
		"idx_results_target_id_id", "idx_results_status_id", "idx_results_engine_id",
	} {
		var got string
		err := s.Read.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&got)
		if err != nil {
			t.Errorf("index %s missing: %v", name, err)
		}
	}
	var applied int
	if err := s.Read.QueryRow(
		`SELECT count(*) FROM schema_migrations WHERE version='0002_result_id_indexes'`).Scan(&applied); err != nil {
		t.Fatalf("query: %v", err)
	}
	if applied != 1 {
		t.Errorf("migration 0002 applied rows = %d, want 1", applied)
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
