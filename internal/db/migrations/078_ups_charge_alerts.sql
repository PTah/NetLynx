-- UPS: пороги заряда, быстрый разряд, низкая автономия, долгая работа от батареи.

UPDATE notification_settings
SET telegram_event_types = CASE
        WHEN telegram_event_types IS NULL OR btrim(telegram_event_types) = '' THEN telegram_event_types
        WHEN upper(telegram_event_types) LIKE '%UPS_CHARGE_LOW%' THEN telegram_event_types
        ELSE telegram_event_types || ',UPS_CHARGE_LOW,UPS_CHARGE_CRITICAL,UPS_CHARGE_OK,UPS_CHARGE_DROP,UPS_DISCHARGE_FAST,UPS_RUNTIME_LOW,UPS_ON_BATTERY_LONG'
    END,
    email_event_types = CASE
        WHEN email_event_types IS NULL OR btrim(email_event_types) = '' THEN email_event_types
        WHEN upper(email_event_types) LIKE '%UPS_CHARGE_LOW%' THEN email_event_types
        ELSE email_event_types || ',UPS_CHARGE_LOW,UPS_CHARGE_CRITICAL,UPS_CHARGE_OK,UPS_CHARGE_DROP,UPS_DISCHARGE_FAST,UPS_RUNTIME_LOW,UPS_ON_BATTERY_LONG'
    END,
    webhook_event_types = CASE
        WHEN webhook_event_types IS NULL OR btrim(webhook_event_types) = '' THEN webhook_event_types
        WHEN upper(webhook_event_types) LIKE '%UPS_CHARGE_LOW%' THEN webhook_event_types
        ELSE webhook_event_types || ',UPS_CHARGE_LOW,UPS_CHARGE_CRITICAL,UPS_CHARGE_OK,UPS_CHARGE_DROP,UPS_DISCHARGE_FAST,UPS_RUNTIME_LOW,UPS_ON_BATTERY_LONG'
    END,
    updated_at = now();

-- Трапы карт ИБП (EPPC/Ippon 935, APC/Schneider 318) — в whitelist trap-логов,
-- чтобы тревоги ИБП были видны в «Логах трапов» без правки настроек вручную.
UPDATE snmp_trap_settings
SET trap_include_labels = CASE
        WHEN trap_include_labels IS NULL OR btrim(trap_include_labels) = '' THEN trap_include_labels
        WHEN upper(trap_include_labels) LIKE '%USTRAPEPPC%' THEN trap_include_labels
        ELSE trap_include_labels || ',upsTrapEppc,upsTrapApc'
    END,
    updated_at = now()
WHERE id = 1;
