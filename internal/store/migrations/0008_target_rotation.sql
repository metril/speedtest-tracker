-- Targets whose engine options carry an ordered list of hosts/server ids
-- (ookla server_ids, iperf3 hosts) round-robin through the list one run at
-- a time. rotation_index is the persisted cursor: NextRotationIndex reads
-- it modulo the list length and advances it, so the choice survives
-- process restarts without writing a target revision.
ALTER TABLE targets ADD COLUMN rotation_index INTEGER NOT NULL DEFAULT 0;
