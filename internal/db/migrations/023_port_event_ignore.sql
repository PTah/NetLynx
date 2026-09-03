-- Ignore list: отключить реакцию (уведомления/действия/события) по порту

CREATE TABLE IF NOT EXISTS port_event_ignore (
    device_id       BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index        INT NOT NULL,
    event_types     TEXT,
    block_events    BOOLEAN NOT NULL DEFAULT false,
    block_notify    BOOLEAN NOT NULL DEFAULT true,
    block_actions   BOOLEAN NOT NULL DEFAULT true,
    comment         TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index)
);

CREATE INDEX IF NOT EXISTS port_event_ignore_device_idx ON port_event_ignore (device_id);
