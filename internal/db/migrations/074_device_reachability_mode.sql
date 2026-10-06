-- Режим определения онлайн/оффлайн:
-- auto    — ping/SNMP (как раньше при online_override IS NULL)
-- ping    — только ICMP ping (IoT без SNMP)
-- online  — вручную онлайн
-- offline — вручную оффлайн
ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS reachability_mode TEXT NOT NULL DEFAULT 'auto';

UPDATE devices
SET reachability_mode = CASE
  WHEN online_override IS TRUE THEN 'online'
  WHEN online_override IS FALSE THEN 'offline'
  ELSE 'auto'
END
WHERE reachability_mode = 'auto'
  AND online_override IS NOT NULL;

ALTER TABLE devices
  DROP CONSTRAINT IF EXISTS devices_reachability_mode_check;

ALTER TABLE devices
  ADD CONSTRAINT devices_reachability_mode_check
  CHECK (reachability_mode IN ('auto', 'ping', 'online', 'offline'));
