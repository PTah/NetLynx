-- Per-device SSH для съёма конфигов (пустое = глобальный fallback из backup_settings).

ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_user TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_password TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_port INT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_enable_password TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS ssh_vendor TEXT NOT NULL DEFAULT 'auto';

UPDATE devices
SET ssh_vendor = 'auto'
WHERE ssh_vendor IS NULL OR btrim(ssh_vendor) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'devices'::regclass AND conname = 'devices_ssh_vendor_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_ssh_vendor_check
            CHECK (ssh_vendor IN ('auto', 'ubiquiti', 'eltex', 'snr'));
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'devices'::regclass AND conname = 'devices_ssh_port_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_ssh_port_check
            CHECK (ssh_port IS NULL OR (ssh_port > 0 AND ssh_port <= 65535));
    END IF;
END $$;
