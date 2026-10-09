-- Отдельный флаг ZIP-бэкапа конфигов роутеров MikroTik (RouterOS /export).
ALTER TABLE backup_settings
  ADD COLUMN IF NOT EXISTS router_cfg_enabled BOOLEAN NOT NULL DEFAULT false;

-- Кто уже снимал конфиги по SSH — оставляем роутеры включёнными (раньше один флаг означал «съём конфигов»).
UPDATE backup_settings
SET router_cfg_enabled = switch_cfg_enabled
WHERE id = 1;
