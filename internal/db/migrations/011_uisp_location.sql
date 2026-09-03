-- UISP: настройки интеграции и расположение узла (site в UISP)

ALTER TABLE devices ADD COLUMN IF NOT EXISTS location TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS uisp_device_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS devices_uisp_device_id_uidx
    ON devices (uisp_device_id)
    WHERE uisp_device_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS uisp_settings (
    id                 INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled            BOOLEAN NOT NULL DEFAULT false,
    base_url           TEXT,
    api_token          TEXT,
    import_community   TEXT NOT NULL DEFAULT 'public',
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO uisp_settings (id) VALUES (1)
    ON CONFLICT (id) DO NOTHING;
