-- Глобальный корень дерева топологии (режим «Устройства»).
CREATE TABLE IF NOT EXISTS topology_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    root_device_id BIGINT REFERENCES devices (id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO topology_settings (id) VALUES (1)
ON CONFLICT (id) DO NOTHING;
