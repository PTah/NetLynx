-- История перемещений MAC (FDB poll + syslog) для расследователя аномалий.

CREATE TABLE IF NOT EXISTS mac_fdb_moves (
    id              BIGSERIAL PRIMARY KEY,
    mac             TEXT NOT NULL,
    device_id       BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    from_if_index   INT,
    to_if_index     INT,
    vlan_id         INT,
    seen_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    source          TEXT NOT NULL DEFAULT 'fdb_poll'
);

CREATE INDEX IF NOT EXISTS mac_fdb_moves_mac_seen_idx
    ON mac_fdb_moves (mac, seen_at DESC);

CREATE INDEX IF NOT EXISTS mac_fdb_moves_device_seen_idx
    ON mac_fdb_moves (device_id, seen_at DESC);

CREATE INDEX IF NOT EXISTS mac_fdb_moves_seen_idx
    ON mac_fdb_moves (seen_at);
