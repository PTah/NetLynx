-- Telegram: токен бота и chat_id (секреты — только на доверенном сервере и с HTTPS в UI)

ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS telegram_bot_token TEXT;
ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS telegram_chat_id TEXT;
ALTER TABLE notification_settings ADD COLUMN IF NOT EXISTS telegram_enabled BOOLEAN NOT NULL DEFAULT false;
