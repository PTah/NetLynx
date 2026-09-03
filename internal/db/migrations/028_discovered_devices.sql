-- Autodiscovery candidates from LLDP/CDP neighbors

CREATE TABLE IF NOT EXISTS discovered_devices (
    id                        BIGSERIAL PRIMARY KEY,
    identity_key              TEXT NOT NULL,
    remote_sys_name           TEXT,
    remote_chassis_id         TEXT,
    remote_mgmt_addr          TEXT,
    first_seen_from_device_id BIGINT REFERENCES devices (id) ON DELETE SET NULL,
    first_seen_if_index       INT,
    last_seen_from_device_id  BIGINT REFERENCES devices (id) ON DELETE SET NULL,
    last_seen_if_index        INT,
    last_protocol             TEXT,
    status                    TEXT NOT NULL DEFAULT 'new'
        CHECK (status IN ('new', 'ignored', 'added')),
    promoted_device_id        BIGINT REFERENCES devices (id) ON DELETE SET NULL,
    last_seen_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT discovered_devices_identity_key_uniq UNIQUE (identity_key)
);

CREATE INDEX IF NOT EXISTS discovered_devices_status_idx ON discovered_devices (status);
CREATE INDEX IF NOT EXISTS discovered_devices_last_seen_idx ON discovered_devices (last_seen_at DESC);
