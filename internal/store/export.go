package store

import (
	"context"
	"fmt"
	"strings"
)

// EachResult streams every result matching f (newest first) to fn, one row
// at a time. Nothing is accumulated in memory, so a CSV export of a
// million rows costs one row of RAM. Tags come from a correlated
// group_concat instead of a second pass, keeping the stream single-query.
// A non-nil error from fn stops the scan and is returned unchanged.
func (s *Store) EachResult(ctx context.Context, f ResultFilter, fn func(*Result) error) error {
	where, args := resultWhere(f)
	q := `SELECT ` + resultColumns + `,
		COALESCE((SELECT group_concat(t.name, ' ') FROM result_tags rt
			JOIN tags t ON t.id = rt.tag_id WHERE rt.result_id = results.id), '')
		FROM results`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY id DESC`

	rows, err := s.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("stream results: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tags string
		r, err := scanResultWithTags(rows, &tags)
		if err != nil {
			return fmt.Errorf("scan result: %w", err)
		}
		if tags != "" {
			r.Tags = strings.Split(tags, " ")
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}
