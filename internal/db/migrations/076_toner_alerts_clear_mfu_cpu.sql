-- Тонер: типы событий в whitelist уведомлений; у МФУ сбрасываем ложный CPU.

UPDATE devices
SET last_cpu_pct = NULL,
    last_cpu_at = NULL,
    cpu_profile = NULL,
    updated_at = now()
WHERE lower(btrim(device_category)) IN ('mfu', 'мфу', 'принтер', 'printer', 'mfp');

UPDATE notification_settings
SET telegram_event_types = CASE
        WHEN telegram_event_types IS NULL OR btrim(telegram_event_types) = '' THEN telegram_event_types
        WHEN upper(telegram_event_types) LIKE '%TONER_LOW%' THEN telegram_event_types
        ELSE telegram_event_types || ',TONER_LOW,TONER_CRITICAL'
    END,
    email_event_types = CASE
        WHEN email_event_types IS NULL OR btrim(email_event_types) = '' THEN email_event_types
        WHEN upper(email_event_types) LIKE '%TONER_LOW%' THEN email_event_types
        ELSE email_event_types || ',TONER_LOW,TONER_CRITICAL'
    END,
    webhook_event_types = CASE
        WHEN webhook_event_types IS NULL OR btrim(webhook_event_types) = '' THEN webhook_event_types
        WHEN upper(webhook_event_types) LIKE '%TONER_LOW%' THEN webhook_event_types
        ELSE webhook_event_types || ',TONER_LOW,TONER_CRITICAL'
    END,
    updated_at = now();
