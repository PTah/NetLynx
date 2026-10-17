-- Разделение опроса: лёгкий (ping + sysUpTime + IF-MIB статусы/счётчики + CPU) каждый
-- poll_interval_seconds и тяжёлый (LLDP/CDP/ARP/PoE/STP/FDB, роли портов) раз в N секунд.

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS heavy_poll_interval_seconds INT NOT NULL DEFAULT 300,
    ADD COLUMN IF NOT EXISTS last_heavy_poll_at TIMESTAMPTZ;

-- Тяжёлый опрос не чаще раза в минуту.
ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_heavy_poll_interval_chk;
ALTER TABLE devices
    ADD CONSTRAINT devices_heavy_poll_interval_chk
    CHECK (heavy_poll_interval_seconds >= 60);

-- Сразу после миграции тяжёлый опрос выполнится на первом же цикле (last_heavy_poll_at = NULL).
