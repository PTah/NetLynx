-- Подпись порта в NetLynx (не SNMP ifDescr/ifAlias). Опрос её не затирает.

ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS descr_override TEXT;
