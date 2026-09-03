-- Пороги утилизации и интервал FDB на уровне узла (NULL = глобальный env)

ALTER TABLE devices ADD COLUMN IF NOT EXISTS util_high_pct REAL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS util_ok_pct REAL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS fdb_poll_interval_seconds INT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conrelid = 'devices'::regclass AND conname = 'devices_fdb_poll_interval_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_fdb_poll_interval_check
            CHECK (fdb_poll_interval_seconds IS NULL OR (fdb_poll_interval_seconds >= 30 AND fdb_poll_interval_seconds <= 86400));
    END IF;
END$$;
