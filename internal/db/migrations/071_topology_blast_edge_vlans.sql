-- VLAN-on-edge + история rebuild для topology blast cache (NMS 0.8.0).

ALTER TABLE topology_blast_edges
    ADD COLUMN IF NOT EXISTS a_if_index INT,
    ADD COLUMN IF NOT EXISTS b_if_index INT,
    ADD COLUMN IF NOT EXISTS protocols TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS topology_blast_edge_vlans (
    a_device_id BIGINT NOT NULL,
    b_device_id BIGINT NOT NULL,
    vlan_id     INT NOT NULL CHECK (vlan_id >= 1 AND vlan_id <= 4094),
    PRIMARY KEY (a_device_id, b_device_id, vlan_id),
    CHECK (a_device_id < b_device_id),
    FOREIGN KEY (a_device_id, b_device_id)
        REFERENCES topology_blast_edges (a_device_id, b_device_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS topology_blast_edge_vlans_vlan_idx
    ON topology_blast_edge_vlans (vlan_id);

CREATE TABLE IF NOT EXISTS topology_blast_history (
    id               BIGSERIAL PRIMARY KEY,
    rebuilt_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    trigger          TEXT NOT NULL DEFAULT '',
    root_device_id   BIGINT,
    root_source      TEXT NOT NULL DEFAULT '',
    edge_count       INT NOT NULL DEFAULT 0,
    node_count       INT NOT NULL DEFAULT 0,
    vlan_edge_count  INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS topology_blast_history_at_idx
    ON topology_blast_history (rebuilt_at DESC);
