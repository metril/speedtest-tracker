package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const sessionColumns = `id,subject,email,name,groups,is_admin,created_at,expires_at`

// sessionTimeFormat is a fixed-width UTC timestamp format so expires_at
// comparisons (used by DeleteExpiredSessions) remain lexicographically
// correct in SQLite, unlike time.RFC3339Nano's variable-width fraction.
const sessionTimeFormat = "2006-01-02T15:04:05.000000000Z"

// Session is a server-side OIDC login session, keyed by the SHA-256 hash of
// the session cookie's random token (the plaintext token never reaches the
// database).
type Session struct {
	ID        string
	Subject   string
	Email     string
	Name      string
	Groups    []string
	IsAdmin   bool
	CreatedAt time.Time
	ExpiresAt time.Time
}

func scanSession(sc interface{ Scan(...any) error }) (Session, error) {
	var s Session
	var groupsJSON string
	var isAdmin int
	var created, expires string
	if err := sc.Scan(&s.ID, &s.Subject, &s.Email, &s.Name, &groupsJSON, &isAdmin, &created, &expires); err != nil {
		return Session{}, err
	}
	s.IsAdmin = isAdmin != 0
	if err := json.Unmarshal([]byte(groupsJSON), &s.Groups); err != nil {
		return Session{}, fmt.Errorf("decode groups: %w", err)
	}
	ct, err := time.Parse(sessionTimeFormat, created)
	if err != nil {
		return Session{}, fmt.Errorf("parse created_at: %w", err)
	}
	et, err := time.Parse(sessionTimeFormat, expires)
	if err != nil {
		return Session{}, fmt.Errorf("parse expires_at: %w", err)
	}
	s.CreatedAt = ct
	s.ExpiresAt = et
	return s, nil
}

// CreateSession inserts a new session row.
func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	groups := sess.Groups
	if groups == nil {
		groups = []string{}
	}
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("encode groups: %w", err)
	}
	isAdmin := 0
	if sess.IsAdmin {
		isAdmin = 1
	}
	_, err = s.Write.ExecContext(ctx,
		`INSERT INTO sessions(`+sessionColumns+`) VALUES(?,?,?,?,?,?,?,?)`,
		sess.ID, sess.Subject, sess.Email, sess.Name, string(groupsJSON), isAdmin,
		sess.CreatedAt.UTC().Format(sessionTimeFormat), sess.ExpiresAt.UTC().Format(sessionTimeFormat))
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// LookupSession resolves the session owning id. An expired or missing
// session both report ok=false with a nil error.
func (s *Store) LookupSession(ctx context.Context, id string, now time.Time) (Session, bool, error) {
	row := s.Read.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id=?`, id)
	sess, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, fmt.Errorf("lookup session: %w", err)
	}
	if !sess.ExpiresAt.After(now) {
		return Session{}, false, nil
	}
	return sess, true, nil
}

// DeleteSession removes the session, if present. Deleting a missing or
// already-deleted session is not an error.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	if _, err := s.Write.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes every session whose expires_at is at or
// before now, returning the number removed.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.Write.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at<=?`, now.UTC().Format(sessionTimeFormat))
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return n, nil
}
