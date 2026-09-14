-- ListResults filters on target_id/status/engine and always orders by
-- id DESC; these composite indexes let SQLite satisfy both the filter and
-- the ORDER BY from a single index instead of a full table scan + sort.
CREATE INDEX idx_results_target_id_id ON results(target_id, id DESC);
CREATE INDEX idx_results_status_id    ON results(status, id DESC);
CREATE INDEX idx_results_engine_id    ON results(engine, id DESC);
