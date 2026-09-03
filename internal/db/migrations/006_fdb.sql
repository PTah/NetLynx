-- FDB/MAC snapshot per device and last poll timestamp.

ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_fdb_poll_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS device_fdb_entries (
    device_id        BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    mac              TEXT NOT NULL,
    if_index         INT NOT NULL,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, mac)
);

CREATE INDEX IF NOT EXISTS device_fdb_entries_device_idx ON device_fdb_entries (device_id);
