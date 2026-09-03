DO $$
DECLARE
    existing_name text;
BEGIN
    SELECT conname
      INTO existing_name
      FROM pg_constraint
     WHERE conrelid = 'devices'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) ILIKE '%snmp_version%';

    IF existing_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE devices DROP CONSTRAINT %I', existing_name);
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conrelid = 'devices'::regclass
           AND conname = 'devices_snmp_version_check'
    ) THEN
        ALTER TABLE devices
            ADD CONSTRAINT devices_snmp_version_check
            CHECK (snmp_version IN ('v1', 'v2c', 'v3'));
    END IF;
END$$;
