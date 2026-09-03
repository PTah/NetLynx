-- Опциональные действия при инцидентах (по умолчанию выключено)

ALTER TABLE notification_settings
    ADD COLUMN IF NOT EXISTS incident_action_enabled BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE notification_settings
    ADD COLUMN IF NOT EXISTS incident_action_event_types TEXT;

UPDATE notification_settings
   SET incident_action_event_types = 'UNKNOWN_MAC_ON_ACCESS_PORT'
 WHERE incident_action_event_types IS NULL;
