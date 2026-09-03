-- Последнее известное состояние STP (BRIDGE-MIB) для детекта topology change.

CREATE TABLE IF NOT EXISTS device_stp_state (
    device_id        BIGINT PRIMARY KEY REFERENCES devices (id) ON DELETE CASCADE,
    top_changes      BIGINT NOT NULL DEFAULT 0,
    designated_root  TEXT,
    root_port        INT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
