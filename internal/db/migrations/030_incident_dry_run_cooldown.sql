-- Incident actions: dry-run + cooldown

ALTER TABLE notification_settings
    ADD COLUMN IF NOT EXISTS incident_action_dry_run BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS incident_action_cooldown_seconds INT NOT NULL DEFAULT 300;

CREATE TABLE IF NOT EXISTS incident_action_cooldowns (
    device_id   BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index    INT NOT NULL,
    action_type TEXT NOT NULL DEFAULT 'admin_down',
    last_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index, action_type)
);
