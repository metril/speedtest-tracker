package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

func TestQueueCRUD(t *testing.T) {
	s, ctx := openTemp(t), context.Background()

	q, err := s.CreateQueue(ctx, "office")
	if err != nil || q.ID == 0 || q.Name != "office" {
		t.Fatalf("CreateQueue: %+v %v", q, err)
	}

	list, err := s.ListQueues(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("ListQueues: %d %v", len(list), err)
	}

	renamed, err := s.RenameQueue(ctx, q.ID, "branch-office")
	if err != nil || renamed.Name != "branch-office" {
		t.Fatalf("RenameQueue: %+v %v", renamed, err)
	}

	if err := s.DeleteQueue(ctx, q.ID); err != nil {
		t.Fatalf("DeleteQueue: %v", err)
	}
	if _, err := s.GetQueue(ctx, q.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetQueue after delete = %v, want ErrNotFound", err)
	}
}

func TestCreateQueueNameConflict(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	if _, err := s.CreateQueue(ctx, "wan"); !errors.Is(err, ErrNameConflict) {
		t.Errorf("CreateQueue duplicate = %v, want ErrNameConflict", err)
	}
}

func TestDeleteQueueInUse(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	wan, err := s.GetQueueByName(ctx, "wan")
	if err != nil {
		t.Fatalf("GetQueueByName: %v", err)
	}
	if _, err := s.CreateTarget(ctx, &Target{Name: "t", Engine: "fake", Enabled: true, QueueID: wan.ID}); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if err := s.DeleteQueue(ctx, wan.ID); !errors.Is(err, ErrQueueInUse) {
		t.Errorf("DeleteQueue in use = %v, want ErrQueueInUse", err)
	}
}

func TestDeleteLastQueueRefused(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	queues, err := s.ListQueues(ctx)
	if err != nil {
		t.Fatalf("ListQueues: %v", err)
	}
	for _, q := range queues[:len(queues)-1] {
		if err := s.DeleteQueue(ctx, q.ID); err != nil {
			t.Fatalf("DeleteQueue %d: %v", q.ID, err)
		}
	}
	last := queues[len(queues)-1]
	if err := s.DeleteQueue(ctx, last.ID); !errors.Is(err, ErrLastQueue) {
		t.Errorf("DeleteQueue last = %v, want ErrLastQueue", err)
	}
}

func TestDefaultQueueIDIsWan(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	id, err := s.DefaultQueueID(ctx)
	if err != nil {
		t.Fatalf("DefaultQueueID: %v", err)
	}
	wan, err := s.GetQueueByName(ctx, "wan")
	if err != nil {
		t.Fatalf("GetQueueByName: %v", err)
	}
	if id != wan.ID {
		t.Errorf("DefaultQueueID = %d, want wan's id %d", id, wan.ID)
	}
}

// TestResolveSnapshotQueueIDLegacyLane covers restoring/reverting a target
// whose revision snapshot predates queues: it carries only "lane", which
// must resolve to the matching queue (falling back to the default queue
// when the name is unknown).
func TestResolveSnapshotQueueIDLegacyLane(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	lan, err := s.GetQueueByName(ctx, "lan")
	if err != nil {
		t.Fatalf("GetQueueByName: %v", err)
	}

	id, err := s.ResolveSnapshotQueueID(ctx, json.RawMessage(`{"lane":"lan"}`))
	if err != nil || id != lan.ID {
		t.Errorf("ResolveSnapshotQueueID(lan) = %d, %v, want %d", id, err, lan.ID)
	}

	def, err := s.DefaultQueueID(ctx)
	if err != nil {
		t.Fatalf("DefaultQueueID: %v", err)
	}
	id, err = s.ResolveSnapshotQueueID(ctx, json.RawMessage(`{"lane":"unknown-lane"}`))
	if err != nil || id != def {
		t.Errorf("ResolveSnapshotQueueID(unknown) = %d, %v, want default %d", id, err, def)
	}

	id, err = s.ResolveSnapshotQueueID(ctx, json.RawMessage(`{"queue_id":`+itoa64(lan.ID)+`}`))
	if err != nil || id != lan.ID {
		t.Errorf("ResolveSnapshotQueueID(queue_id) = %d, %v, want %d", id, err, lan.ID)
	}
}

func itoa64(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// TestMigrationBackfillsQueueIDFromLane applies migrations 0001-0006 by
// hand (the pre-queues schema), inserts targets against the old lane
// column directly -- including a lane other than wan/lan, to prove that
// case is seeded too -- then applies the rest (0007+) and checks queues
// were seeded and every target's queue_id matches its original lane.
func TestMigrationBackfillsQueueIDFromLane(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/legacy.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, name := range []string{
		"0001_init.sql", "0002_result_id_indexes.sql", "0003_api_tokens.sql",
		"0004_iperf3_servers.sql", "0005_target_revisions.sql", "0006_iperf3_port_end.sql",
	} {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		version := name[:len(name)-len(".sql")]
		if _, err := db.ExecContext(ctx,
			`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}

	for _, tc := range []struct{ name, lane string }{
		{"office", "wan"}, {"nas", "lan"}, {"branch", "dmz"},
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO targets(name,engine,lane) VALUES(?,?,?)`, tc.name, "fake", tc.lane); err != nil {
			t.Fatalf("insert %s: %v", tc.name, err)
		}
	}

	if err := migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	queueNames := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT name FROM queues`)
	if err != nil {
		t.Fatalf("list queues: %v", err)
	}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		queueNames[n] = true
	}
	rows.Close()
	for _, want := range []string{"wan", "lan", "dmz"} {
		if !queueNames[want] {
			t.Errorf("queues missing %q, got %v", want, queueNames)
		}
	}

	for _, tc := range []struct{ name, lane string }{
		{"office", "wan"}, {"nas", "lan"}, {"branch", "dmz"},
	} {
		var queueName string
		if err := db.QueryRowContext(ctx,
			`SELECT q.name FROM targets t JOIN queues q ON q.id=t.queue_id WHERE t.name=?`, tc.name,
		).Scan(&queueName); err != nil {
			t.Fatalf("scan %s: %v", tc.name, err)
		}
		if queueName != tc.lane {
			t.Errorf("target %s queue = %q, want %q", tc.name, queueName, tc.lane)
		}
	}
}
