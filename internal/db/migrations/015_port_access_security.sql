-- Состояние «памяти» access-портов: привязанный MAC и момент, когда порт стал пустым (для FDB-событий безопасности).

CREATE TABLE IF NOT EXISTS port_access_security (
    device_id   BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index    INT NOT NULL,
    bound_mac   TEXT,
    empty_since TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index)
);

CREATE INDEX IF NOT EXISTS port_access_security_device_idx ON port_access_security (device_id);
