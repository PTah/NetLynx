-- Manual topology links (not in port_neighbors — poller must not stale/delete them)

CREATE TABLE IF NOT EXISTS manual_topology_links (
    id              BIGSERIAL PRIMARY KEY,
    a_device_id     BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    a_if_index      INT NOT NULL CHECK (a_if_index > 0),
    b_device_id     BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    b_if_index      INT NOT NULL CHECK (b_if_index > 0),
    note            TEXT,
    status          TEXT NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'superseded')),
    superseded_at   TIMESTAMPTZ,
    superseded_by   TEXT,
    created_by      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT manual_topology_links_distinct_devices CHECK (a_device_id <> b_device_id),
    CONSTRAINT manual_topology_links_canonical CHECK (a_device_id < b_device_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS manual_topology_links_pair_uidx
    ON manual_topology_links (a_device_id, a_if_index, b_device_id, b_if_index);

CREATE INDEX IF NOT EXISTS manual_topology_links_a_idx ON manual_topology_links (a_device_id);
CREATE INDEX IF NOT EXISTS manual_topology_links_b_idx ON manual_topology_links (b_device_id);
CREATE INDEX IF NOT EXISTS manual_topology_links_status_idx ON manual_topology_links (status);
