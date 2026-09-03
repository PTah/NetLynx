-- Статус из UISP overview.status (active / disconnected и т.д.) для фильтра «онлайн» на дашборде

ALTER TABLE devices ADD COLUMN IF NOT EXISTS uisp_overview_status TEXT;

UPDATE devices SET uisp_overview_status = 'active'
WHERE uisp_device_id IS NOT NULL AND uisp_overview_status IS NULL;
