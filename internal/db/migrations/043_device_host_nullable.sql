-- Host можно очистить (склад / смена адреса позже). Несколько узлов без IP — ок.
ALTER TABLE devices ALTER COLUMN host DROP NOT NULL;

DROP INDEX IF EXISTS devices_host_lower_uidx;
CREATE UNIQUE INDEX IF NOT EXISTS devices_host_lower_uidx
  ON devices (lower(btrim(host)))
  WHERE host IS NOT NULL AND btrim(host) <> '';
