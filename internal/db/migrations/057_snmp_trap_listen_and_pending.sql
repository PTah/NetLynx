-- Приём traps (UI) и ожидание подтверждения linkUp/linkDown опросом.
ALTER TABLE snmp_trap_settings
    ADD COLUMN IF NOT EXISTS listen_enabled BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE snmp_trap_settings
    ADD COLUMN IF NOT EXISTS listen_port INT NOT NULL DEFAULT 9162;

UPDATE snmp_trap_settings
SET listen_enabled = true,
    listen_port = 9162
WHERE id = 1 AND (listen_port IS NULL OR listen_port < 1 OR listen_port > 65535);

CREATE TABLE IF NOT EXISTS snmp_trap_pending_link (
    device_id BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index INT NOT NULL,
    expected_oper SMALLINT NOT NULL CHECK (expected_oper IN (1, 2)),
    trap_label TEXT,
    source_ip TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index)
);

CREATE INDEX IF NOT EXISTS snmp_trap_pending_link_received_at_idx
    ON snmp_trap_pending_link (received_at);
