package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const apiTokenColumns = `id,name,prefix,created_at,last_used_at`

// APIToken is one issued API token. The plaintext token is never stored
// and never leaves the process after creation: hash is a hex SHA-256
// digest, and Prefix is the human-recognisable leading fragment shown in
// the UI. The hash has no JSON tag on purpose — this struct is serialised
// straight onto the wire.
type APIToken struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

func scanAPIToken(sc interface{ Scan(...any) error }) (APIToken, error) {
	var t APIToken
	var lastUsed sql.NullString
	if err := sc.Scan(&t.ID, &t.Name, &t.Prefix, &t.CreatedAt, &lastUsed); err != nil {
		return APIToken{}, err
	}
	t.LastUsedAt = lastUsed.String
	return t, nil
}

// CreateAPIToken inserts a new token row and returns it.
func (s *Store) CreateAPIToken(ctx context.Context, name, hash, prefix string) (APIToken, error) {
	res, err := s.Write.ExecContext(ctx,
		`INSERT INTO api_tokens(name,hash,prefix) VALUES(?,?,?)`, name, hash, prefix)
	if err != nil {
		return APIToken{}, fmt.Errorf("insert api token: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIToken{}, fmt.Errorf("insert api token: %w", err)
	}
	row := s.Write.QueryRowContext(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE id=?`, id)
	t, err := scanAPIToken(row)
	if err != nil {
		return APIToken{}, fmt.Errorf("read created api token: %w", err)
	}
	return t, nil
}

// ListAPITokens returns every token, newest first. The hash is never
// selected, so it can never leak onto the wire through this path.
func (s *Store) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.Read.QueryContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	out := []APIToken{}
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	return out, nil
}

// DeleteAPIToken removes the token, or returns ErrNotFound.
func (s *Store) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := s.Write.ExecContext(ctx, `DELETE FROM api_tokens WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete api token %d: %w", id, err)
	}
	return requireAffected(res)
}

// APITokenByHash resolves the token owning hash. This is the middleware's
// hot lookup path: ok is false, with a nil error, when no token matches.
func (s *Store) APITokenByHash(ctx context.Context, hash string) (APIToken, bool, error) {
	row := s.Read.QueryRowContext(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE hash=?`, hash)
	t, err := scanAPIToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return APIToken{}, false, nil
	}
	if err != nil {
		return APIToken{}, false, fmt.Errorf("api token by hash: %w", err)
	}
	return t, true, nil
}

// TouchAPIToken stamps last_used_at with the current time.
func (s *Store) TouchAPIToken(ctx context.Context, id int64) error {
	res, err := s.Write.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("touch api token %d: %w", id, err)
	}
	return requireAffected(res)
}

// CountAPITokens returns the number of issued tokens.
func (s *Store) CountAPITokens(ctx context.Context) (int, error) {
	var n int
	if err := s.Read.QueryRowContext(ctx, `SELECT count(*) FROM api_tokens`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count api tokens: %w", err)
	}
	return n, nil
}
