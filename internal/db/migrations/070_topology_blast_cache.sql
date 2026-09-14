-- Кэш L2-топологии для VLAN blast-radius (adj / root / dist).
-- Пересборка по расписанию (см. TOPOLOGY_BLAST_CACHE_*); UI /topology по-прежнему live.

CREATE TABLE IF NOT EXISTS topology_blast_meta (
    id              INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    root_device_id  BIGINT,
    root_source     TEXT NOT NULL DEFAULT '',
    edge_count      INT NOT NULL DEFAULT 0,
    node_count      INT NOT NULL DEFAULT 0,
    rebuilt_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO topology_blast_meta (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS topology_blast_edges (
    a_device_id BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    b_device_id BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    PRIMARY KEY (a_device_id, b_device_id),
    CHECK (a_device_id < b_device_id)
);

CREATE INDEX IF NOT EXISTS topology_blast_edges_b_idx
    ON topology_blast_edges (b_device_id);

CREATE TABLE IF NOT EXISTS topology_blast_dist (
    device_id BIGINT PRIMARY KEY REFERENCES devices (id) ON DELETE CASCADE,
    dist      INT NOT NULL
);
