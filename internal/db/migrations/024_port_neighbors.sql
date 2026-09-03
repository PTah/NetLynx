-- Autodiscover: сосед по LLDP на локальном порту (MVP — одна запись на if_index)

CREATE TABLE IF NOT EXISTS port_neighbors (
    device_id           BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index            INT NOT NULL,
    protocol            TEXT NOT NULL DEFAULT 'lldp',
    remote_sys_name     TEXT,
    remote_port_id      TEXT,
    remote_chassis_id   TEXT,
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index, protocol)
);

CREATE INDEX IF NOT EXISTS port_neighbors_device_idx ON port_neighbors (device_id);
