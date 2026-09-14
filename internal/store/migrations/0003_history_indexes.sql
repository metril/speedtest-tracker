-- HistoryBuckets, Summary and Outages all scan one target's results over a
-- time window; this composite index satisfies the filter and the ordering
-- without touching the table until the aggregate needs a column.
CREATE INDEX IF NOT EXISTS idx_results_target_started ON results(target_id, started_at);
-- Outages scans failures across every target over a window.
CREATE INDEX IF NOT EXISTS idx_results_status_started ON results(status, started_at);
