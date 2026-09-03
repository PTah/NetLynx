-- Уникальность host (без учёта регистра и пробелов по краям).
CREATE UNIQUE INDEX IF NOT EXISTS devices_host_lower_uidx
  ON devices (lower(btrim(host)));
