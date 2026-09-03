-- Уведомления: webhook (расширяемо позже — email, Telegram)

CREATE TABLE IF NOT EXISTS notification_settings (
    id                SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    webhook_url       TEXT,
    webhook_enabled   BOOLEAN NOT NULL DEFAULT false,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO notification_settings (id, webhook_enabled)
VALUES (1, false)
ON CONFLICT (id) DO NOTHING;
