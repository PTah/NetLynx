-- История running-config свитчей (SSH) для diff и расследования «что изменилось».

CREATE TABLE IF NOT EXISTS device_config_snapshots (
    id           BIGSERIAL PRIMARY KEY,
    device_id    BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    config_text  TEXT NOT NULL,
    config_hash  TEXT NOT NULL,
    source       TEXT NOT NULL DEFAULT 'scheduled'
);

CREATE INDEX IF NOT EXISTS device_config_snapshots_device_fetched_idx
    ON device_config_snapshots (device_id, fetched_at DESC);

CREATE INDEX IF NOT EXISTS device_config_snapshots_fetched_idx
    ON device_config_snapshots (fetched_at);
