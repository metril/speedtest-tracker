package store

import (
	"context"
	"fmt"
)

// ReapOrphanedRuns marks every queued/running run failed. It is meant to run
// once at startup, before the runner or scheduler start: any such row was
// left behind by a process that died mid-run and would otherwise block the
// schedule (and refuse cancellation) forever. It returns the number reaped.
func (s *Store) ReapOrphanedRuns(ctx context.Context) (int64, error) {
	res, err := s.Write.ExecContext(ctx,
		`UPDATE runs SET status='failed', error='interrupted by restart', finished_at=`+nowExpr+
			` WHERE status IN ('queued','running')`)
	if err != nil {
		return 0, fmt.Errorf("reap orphaned runs: %w", err)
	}
	return res.RowsAffected()
}
