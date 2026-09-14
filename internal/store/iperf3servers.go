package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const iperf3ServerColumns = `id,host,port,options,supports_reverse,supports_udp,gbs,continent,country,site,provider`

// defaultIperf3SearchLimit bounds SearchIperf3Servers when the caller
// passes a non-positive limit, so a stray zero can never turn into an
// unbounded query.
const defaultIperf3SearchLimit = 50

// Iperf3Server is one row of the cached public iperf3 server list
// (fetched from export.iperf3serverlist.net; see internal/iperf3list).
type Iperf3Server struct {
	ID              int64  `json:"id"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Options         string `json:"options,omitempty"`
	SupportsReverse bool   `json:"supports_reverse"`
	SupportsUDP     bool   `json:"supports_udp"`
	GBs             string `json:"gbs,omitempty"`
	Continent       string `json:"continent,omitempty"`
	Country         string `json:"country,omitempty"`
	Site            string `json:"site,omitempty"`
	Provider        string `json:"provider,omitempty"`
}

func scanIperf3Server(sc interface{ Scan(...any) error }) (Iperf3Server, error) {
	var s Iperf3Server
	if err := sc.Scan(&s.ID, &s.Host, &s.Port, &s.Options, &s.SupportsReverse, &s.SupportsUDP,
		&s.GBs, &s.Continent, &s.Country, &s.Site, &s.Provider); err != nil {
		return Iperf3Server{}, err
	}
	return s, nil
}

// ReplaceIperf3Servers atomically swaps the cached server list: it deletes
// every existing row, inserts servers (silently ignoring any row that
// collides with another on the UNIQUE(host,port) constraint, so one
// duplicate never fails the whole batch), and stamps iperf3_meta's
// fetched_at with the current time — all inside one transaction.
//
// It always does exactly what it's told, including wiping the table when
// servers is empty: a caller that wants to keep the existing table on an
// empty fetch (see internal/iperf3list.Refresher) must simply not call
// this with an empty slice.
func (s *Store) ReplaceIperf3Servers(ctx context.Context, servers []Iperf3Server) error {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace iperf3 servers: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM iperf3_servers`); err != nil {
		return fmt.Errorf("clear iperf3 servers: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO iperf3_servers
			(host,port,options,supports_reverse,supports_udp,gbs,continent,country,site,provider)
		VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare insert iperf3 server: %w", err)
	}
	defer stmt.Close()

	for _, srv := range servers {
		if _, err := stmt.ExecContext(ctx, srv.Host, srv.Port, srv.Options, srv.SupportsReverse, srv.SupportsUDP,
			srv.GBs, srv.Continent, srv.Country, srv.Site, srv.Provider); err != nil {
			return fmt.Errorf("insert iperf3 server %s:%d: %w", srv.Host, srv.Port, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO iperf3_meta(k,v) VALUES('fetched_at', strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v`); err != nil {
		return fmt.Errorf("stamp iperf3 fetched_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace iperf3 servers: %w", err)
	}
	return nil
}

// SearchIperf3Servers returns up to limit servers (defaultIperf3SearchLimit
// when limit <= 0), ordered by host/port. An empty q returns the first
// rows unfiltered; a non-empty q matches (case-insensitively) against
// host, site, country or provider.
func (s *Store) SearchIperf3Servers(ctx context.Context, q string, limit int) ([]Iperf3Server, error) {
	if limit <= 0 {
		limit = defaultIperf3SearchLimit
	}

	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = s.Read.QueryContext(ctx,
			`SELECT `+iperf3ServerColumns+` FROM iperf3_servers ORDER BY host, port LIMIT ?`, limit)
	} else {
		like := "%" + q + "%"
		rows, err = s.Read.QueryContext(ctx, `
			SELECT `+iperf3ServerColumns+` FROM iperf3_servers
			WHERE host LIKE ? ESCAPE '\' COLLATE NOCASE
			   OR site LIKE ? ESCAPE '\' COLLATE NOCASE
			   OR country LIKE ? ESCAPE '\' COLLATE NOCASE
			   OR provider LIKE ? ESCAPE '\' COLLATE NOCASE
			ORDER BY host, port LIMIT ?`, like, like, like, like, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search iperf3 servers: %w", err)
	}
	defer rows.Close()

	out := []Iperf3Server{}
	for rows.Next() {
		srv, err := scanIperf3Server(rows)
		if err != nil {
			return nil, fmt.Errorf("scan iperf3 server: %w", err)
		}
		out = append(out, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search iperf3 servers: %w", err)
	}
	return out, nil
}

// CountIperf3Servers returns the total number of cached servers,
// regardless of any search filter.
func (s *Store) CountIperf3Servers(ctx context.Context) (int, error) {
	var n int
	if err := s.Read.QueryRowContext(ctx, `SELECT count(*) FROM iperf3_servers`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count iperf3 servers: %w", err)
	}
	return n, nil
}

// Iperf3ServersFetchedAt returns the timestamp of the most recent
// successful refresh, or "" if the list has never been fetched.
func (s *Store) Iperf3ServersFetchedAt(ctx context.Context) (string, error) {
	var v string
	err := s.Read.QueryRowContext(ctx, `SELECT v FROM iperf3_meta WHERE k='fetched_at'`).Scan(&v)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("read iperf3 fetched_at: %w", err)
	}
	return v, nil
}
