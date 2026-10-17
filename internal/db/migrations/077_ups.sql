-- ИБП (UPS-MIB, RFC 1628): снимок статуса на узле + история в metric_samples (ups_*).

ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_ups JSONB;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_ups_at TIMESTAMPTZ;

-- Тип узла «ИБП» (sort_order 75 — между МФУ 70 и камерами 80).
INSERT INTO device_category_defs (id, label, color, builtin, sort_order) VALUES
    ('ups', 'ИБП', '#a0c4ff', TRUE, 75)
ON CONFLICT (id) DO NOTHING;

-- UPS-события в whitelist уведомлений (как TONER_* в 076).
UPDATE notification_settings
SET telegram_event_types = CASE
        WHEN telegram_event_types IS NULL OR btrim(telegram_event_types) = '' THEN telegram_event_types
        WHEN upper(telegram_event_types) LIKE '%UPS_ON_BATTERY%' THEN telegram_event_types
        ELSE telegram_event_types || ',UPS_ON_BATTERY,UPS_MAINS_OK,UPS_BATTERY_LOW,UPS_BATTERY_DEPLETED,UPS_BATTERY_OK,UPS_TEMP_HIGH,UPS_TEMP_CRITICAL,UPS_ALARM,UPS_ALARM_CLEAR,UPS_LOAD_HIGH,UPS_LOAD_OK'
    END,
    email_event_types = CASE
        WHEN email_event_types IS NULL OR btrim(email_event_types) = '' THEN email_event_types
        WHEN upper(email_event_types) LIKE '%UPS_ON_BATTERY%' THEN email_event_types
        ELSE email_event_types || ',UPS_ON_BATTERY,UPS_MAINS_OK,UPS_BATTERY_LOW,UPS_BATTERY_DEPLETED,UPS_BATTERY_OK,UPS_TEMP_HIGH,UPS_TEMP_CRITICAL,UPS_ALARM,UPS_ALARM_CLEAR,UPS_LOAD_HIGH,UPS_LOAD_OK'
    END,
    webhook_event_types = CASE
        WHEN webhook_event_types IS NULL OR btrim(webhook_event_types) = '' THEN webhook_event_types
        WHEN upper(webhook_event_types) LIKE '%UPS_ON_BATTERY%' THEN webhook_event_types
        ELSE webhook_event_types || ',UPS_ON_BATTERY,UPS_MAINS_OK,UPS_BATTERY_LOW,UPS_BATTERY_DEPLETED,UPS_BATTERY_OK,UPS_TEMP_HIGH,UPS_TEMP_CRITICAL,UPS_ALARM,UPS_ALARM_CLEAR,UPS_LOAD_HIGH,UPS_LOAD_OK'
    END,
    updated_at = now();
