-- Multi-neighbor (rem_index), stale TTL, optional mgmt addr (CDP)

ALTER TABLE port_neighbors ADD COLUMN IF NOT EXISTS rem_index INT NOT NULL DEFAULT 1;
ALTER TABLE port_neighbors ADD COLUMN IF NOT EXISTS remote_mgmt_addr TEXT;
ALTER TABLE port_neighbors ADD COLUMN IF NOT EXISTS stale BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE port_neighbors DROP CONSTRAINT IF EXISTS port_neighbors_pkey;
ALTER TABLE port_neighbors ADD PRIMARY KEY (device_id, if_index, protocol, rem_index);

CREATE INDEX IF NOT EXISTS port_neighbors_last_seen_idx ON port_neighbors (last_seen_at);
CREATE INDEX IF NOT EXISTS port_neighbors_stale_idx ON port_neighbors (device_id, stale);
