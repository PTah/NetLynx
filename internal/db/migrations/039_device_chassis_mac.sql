-- Chassis MAC устройства (LLDP loc / bridge address) для резолва соседей MAC → inventory.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS chassis_mac TEXT;

-- Уникальность по нормализованному hex (без разделителей), только для заполненных.
CREATE UNIQUE INDEX IF NOT EXISTS devices_chassis_mac_hex_uidx
  ON devices (
    lower(replace(replace(chassis_mac, ':', ''), '-', ''))
  )
  WHERE chassis_mac IS NOT NULL AND btrim(chassis_mac) <> '';
