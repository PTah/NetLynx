-- Ежедневные снимки FDB для «где MAC был N дней назад» (дополнение к mac_fdb_moves).

CREATE TABLE IF NOT EXISTS fdb_snapshots (
    id           BIGSERIAL PRIMARY KEY,
    device_id    BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    snapshot_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    entry_count  INT NOT NULL DEFAULT 0,
    source       TEXT NOT NULL DEFAULT 'fdb_poll'
);

CREATE INDEX IF NOT EXISTS fdb_snapshots_device_at_idx
    ON fdb_snapshots (device_id, snapshot_at DESC);

CREATE INDEX IF NOT EXISTS fdb_snapshots_at_idx
    ON fdb_snapshots (snapshot_at);

CREATE TABLE IF NOT EXISTS fdb_snapshot_entries (
    snapshot_id  BIGINT NOT NULL REFERENCES fdb_snapshots (id) ON DELETE CASCADE,
    mac          TEXT NOT NULL,
    if_index     INT NOT NULL,
    vlan_id      INT,
    PRIMARY KEY (snapshot_id, mac)
);

CREATE INDEX IF NOT EXISTS fdb_snapshot_entries_mac_idx
    ON fdb_snapshot_entries (mac);
