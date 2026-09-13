package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Tag is a free-form label attachable to results.
type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ListTags returns every tag, ordered by name.
func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.Read.QueryContext(ctx, `SELECT id,name FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// normaliseTagNames trims, lowercases, drops empties and dedupes, sorted.
func normaliseTagNames(names []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// SetResultTags replaces the tag set of a result, creating missing tag
// rows. It returns the stored (normalised) names, or ErrNotFound when the
// result does not exist. One write transaction.
func (s *Store) SetResultTags(ctx context.Context, resultID int64, names []string) ([]string, error) {
	want := normaliseTagNames(names)

	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tags tx: %w", err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM results WHERE id=?`, resultID).Scan(&exists); err != nil {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM result_tags WHERE result_id=?`, resultID); err != nil {
		return nil, fmt.Errorf("clear result tags: %w", err)
	}
	for _, name := range want {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tags(name) VALUES(?) ON CONFLICT(name) DO NOTHING`, name); err != nil {
			return nil, fmt.Errorf("insert tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO result_tags(result_id,tag_id)
			SELECT ?, id FROM tags WHERE name=?`, resultID, name); err != nil {
			return nil, fmt.Errorf("attach tag %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tags: %w", err)
	}
	return want, nil
}

// TagsForResults returns the tag names of each result id, sorted.
func (s *Store) TagsForResults(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.Read.QueryContext(ctx, `
		SELECT rt.result_id, t.name FROM result_tags rt
		JOIN tags t ON t.id = rt.tag_id
		WHERE rt.result_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)
		ORDER BY t.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("tags for results: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan result tag: %w", err)
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

// attachTags fills in the Tags field of each result in one extra query.
func (s *Store) attachTags(ctx context.Context, results []Result) error {
	if len(results) == 0 {
		return nil
	}
	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	byID, err := s.TagsForResults(ctx, ids)
	if err != nil {
		return err
	}
	for i := range results {
		if tags, ok := byID[results[i].ID]; ok {
			results[i].Tags = tags
		}
	}
	return nil
}
