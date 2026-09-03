-- Ускоряет LookupMACInFDBSnapshots (WHERE lower(replace(replace(mac…))) = $1).
CREATE INDEX IF NOT EXISTS fdb_snapshot_entries_mac_hex_idx
    ON fdb_snapshot_entries (lower(replace(replace(mac, ':', ''), '-', '')));
