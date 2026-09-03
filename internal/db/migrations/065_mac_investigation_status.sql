-- Статус расследования MAC (open / resolved / ignored) для операторов.

CREATE TABLE IF NOT EXISTS mac_investigation_status (
    mac              TEXT PRIMARY KEY,
    status           TEXT NOT NULL DEFAULT 'open',
    note             TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by       BIGINT REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS mac_investigation_status_status_idx
    ON mac_investigation_status (status, updated_at DESC);

ALTER TABLE mac_investigation_status
    ADD CONSTRAINT mac_investigation_status_status_check
    CHECK (status IN ('open', 'resolved', 'ignored'));
