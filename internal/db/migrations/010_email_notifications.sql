-- Email notification channel settings (SMTP).

ALTER TABLE notification_settings
    ADD COLUMN IF NOT EXISTS email_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS email_from TEXT,
    ADD COLUMN IF NOT EXISTS email_to TEXT,
    ADD COLUMN IF NOT EXISTS email_event_types TEXT,
    ADD COLUMN IF NOT EXISTS email_severities TEXT,
    ADD COLUMN IF NOT EXISTS smtp_host TEXT,
    ADD COLUMN IF NOT EXISTS smtp_port INT NOT NULL DEFAULT 587,
    ADD COLUMN IF NOT EXISTS smtp_username TEXT,
    ADD COLUMN IF NOT EXISTS smtp_password TEXT;
