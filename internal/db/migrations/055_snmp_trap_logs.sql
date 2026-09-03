-- Настройки и тестовый журнал входящих SNMP traps.
CREATE TABLE IF NOT EXISTS snmp_trap_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    log_enabled BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO snmp_trap_settings (id, log_enabled)
VALUES (1, false)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS snmp_trap_logs (
    id BIGSERIAL PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    source_ip TEXT NOT NULL,
    device_id BIGINT REFERENCES devices (id) ON DELETE SET NULL,
    snmp_version TEXT,
    community TEXT,
    trap_oid TEXT,
    if_index INT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS snmp_trap_logs_received_at_idx
    ON snmp_trap_logs (received_at DESC);
