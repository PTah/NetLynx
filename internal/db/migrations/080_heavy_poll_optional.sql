-- Тяжёлый опрос: NULL = «как в POLL_HEAVY_SECONDS» (по умолчанию 300 с), значение задаётся в карточке узла.

ALTER TABLE devices ALTER COLUMN heavy_poll_interval_seconds DROP NOT NULL;
ALTER TABLE devices ALTER COLUMN heavy_poll_interval_seconds DROP DEFAULT;

ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_heavy_poll_interval_chk;
ALTER TABLE devices
    ADD CONSTRAINT devices_heavy_poll_interval_chk
    CHECK (heavy_poll_interval_seconds IS NULL OR heavy_poll_interval_seconds BETWEEN 60 AND 86400);

UPDATE devices SET heavy_poll_interval_seconds = NULL WHERE heavy_poll_interval_seconds = 300;
