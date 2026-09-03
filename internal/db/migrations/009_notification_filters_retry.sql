-- Notification subscriptions and retry/backoff tuning.

ALTER TABLE notification_settings
    ADD COLUMN IF NOT EXISTS webhook_event_types TEXT,
    ADD COLUMN IF NOT EXISTS webhook_severities TEXT,
    ADD COLUMN IF NOT EXISTS telegram_event_types TEXT,
    ADD COLUMN IF NOT EXISTS telegram_severities TEXT,
    ADD COLUMN IF NOT EXISTS notify_max_retries INT NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS notify_retry_backoff_ms INT NOT NULL DEFAULT 500;
