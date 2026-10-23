-- Расписание сброса PoE на порту (время сервера NetLynx).

CREATE TABLE IF NOT EXISTS port_poe_reset_schedule (
    device_id           BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index            INT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT false,
    -- at_time: один раз в сутки в start_*; times_per_day: N раз начиная с start_*
    mode                TEXT NOT NULL DEFAULT 'at_time'
                        CHECK (mode IN ('at_time', 'times_per_day')),
    start_hour          INT NOT NULL DEFAULT 3
                        CHECK (start_hour >= 0 AND start_hour <= 23),
    start_minute        INT NOT NULL DEFAULT 0
                        CHECK (start_minute >= 0 AND start_minute <= 59),
    times_per_day       INT NOT NULL DEFAULT 1
                        CHECK (times_per_day >= 1 AND times_per_day <= 24),
    reset_seconds       INT NOT NULL DEFAULT 10
                        CHECK (reset_seconds >= 1 AND reset_seconds <= 60),
    last_fired_key      TEXT,
    last_run_at         TIMESTAMPTZ,
    last_status         TEXT,
    last_error          TEXT,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, if_index)
);

CREATE INDEX IF NOT EXISTS port_poe_reset_schedule_enabled_idx
    ON port_poe_reset_schedule (enabled)
    WHERE enabled = true;
