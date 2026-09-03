-- SMTP по IP / self-signed: пропуск проверки сертификата.
ALTER TABLE notification_settings
  ADD COLUMN IF NOT EXISTS smtp_tls_skip_verify BOOLEAN NOT NULL DEFAULT false;
