-- NetLynx: начальная схема (узлы, интерфейсы, счётчики, события)

CREATE TABLE IF NOT EXISTS devices (
    id                      BIGSERIAL PRIMARY KEY,
    name                    TEXT NOT NULL,
    host                    TEXT NOT NULL,
    snmp_version            TEXT NOT NULL CHECK (snmp_version IN ('v2c', 'v3')),
    community               TEXT,
    v3_user                 TEXT,
    v3_auth_protocol        TEXT,
    v3_auth_pass            TEXT,
    v3_priv_protocol        TEXT,
    v3_priv_pass            TEXT,
    v3_engine_id            TEXT,
    poll_interval_seconds   INT NOT NULL DEFAULT 60 CHECK (poll_interval_seconds >= 10 AND poll_interval_seconds <= 86400),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_poll_at            TIMESTAMPTZ,
    last_snmp_ok            BOOLEAN,
    last_snmp_error         TEXT,
    sys_name                TEXT,
    sys_descr               TEXT
);

CREATE INDEX IF NOT EXISTS devices_host_idx ON devices (host);

CREATE TABLE IF NOT EXISTS device_interfaces (
    device_id           BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index            INT NOT NULL,
    if_descr            TEXT,
    if_name             TEXT,
    if_type             BIGINT,
    admin_status        INT,
    oper_status         INT,
    if_speed            BIGINT,
    if_high_speed       BIGINT,
    hc_in_octets        BIGINT,
    hc_out_octets       BIGINT,
    counters_polled_at  TIMESTAMPTZ,
    util_in_pct         REAL,
    util_out_pct        REAL,
    util_max_pct        REAL,
    util_high_active    BOOLEAN NOT NULL DEFAULT false,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index)
);

CREATE INDEX IF NOT EXISTS device_interfaces_device_idx ON device_interfaces (device_id);

CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL PRIMARY KEY,
    device_id   BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index    INT,
    event_type  TEXT NOT NULL,
    severity    TEXT NOT NULL DEFAULT 'info',
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS events_device_created_idx ON events (device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS events_created_idx ON events (created_at DESC);
