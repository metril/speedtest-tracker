// Package store owns the SQLite database: connection pools, PRAGMAs and
// schema migrations.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

// Store holds two pools over the same database file. Read is sized to
// NumCPU (WAL allows concurrent readers); Write is capped at one
// connection so writers never collide with SQLITE_BUSY.
type Store struct {
	Read  *sql.DB
	Write *sql.DB
	Path  string
}

// pragmas are appended to the DSN so every new connection in either pool
// gets them, not just the first one.
const pragmas = "?_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=synchronous(1)" +
	"&_txlock=immediate"

// Open opens (creating parent directories and the file as needed) the
// database at path, applies pending migrations and returns the Store.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}
	dsn := "file:" + path + pragmas

	write, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)

	read, err := sql.Open("sqlite", dsn)
	if err != nil {
		write.Close()
		return nil, fmt.Errorf("open read pool: %w", err)
	}
	read.SetMaxOpenConns(runtime.NumCPU())
	read.SetMaxIdleConns(runtime.NumCPU())

	s := &Store{Read: read, Write: write, Path: path}
	if err := migrate(context.Background(), write); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Ping verifies both pools can reach the database.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.Write.PingContext(ctx); err != nil {
		return fmt.Errorf("write pool: %w", err)
	}
	if err := s.Read.PingContext(ctx); err != nil {
		return fmt.Errorf("read pool: %w", err)
	}
	return nil
}

// Close closes both pools.
func (s *Store) Close() error {
	return errors.Join(s.Read.Close(), s.Write.Close())
}
