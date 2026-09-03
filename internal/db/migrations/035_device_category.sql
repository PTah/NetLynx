-- Тип узла для списка «Узлы» и фильтров.
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS device_category TEXT NOT NULL DEFAULT 'switch';

UPDATE devices
SET device_category = 'switch'
WHERE device_category IS NULL OR trim(device_category) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'devices'::regclass AND conname = 'devices_device_category_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_device_category_check
            CHECK (device_category IN ('switch', 'server', 'computer', 'mfu', 'camera', 'other'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS devices_device_category_idx ON devices (device_category);
