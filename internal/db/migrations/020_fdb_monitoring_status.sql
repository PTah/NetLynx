-- Статус MAC/FDB-мониторинга на узле

ALTER TABLE devices ADD COLUMN IF NOT EXISTS fdb_monitoring_status TEXT NOT NULL DEFAULT 'unknown';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conrelid = 'devices'::regclass AND conname = 'devices_fdb_monitoring_status_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_fdb_monitoring_status_check
            CHECK (fdb_monitoring_status IN ('unknown', 'ok', 'learning', 'unavailable'));
    END IF;
END$$;
