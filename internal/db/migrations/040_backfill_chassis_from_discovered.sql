-- Дозаполнить chassis_mac у узлов, добавленных из «Обнаружено» / топологии по LLDP MAC.
UPDATE devices d
SET chassis_mac = lower(btrim(dd.remote_chassis_id))
FROM discovered_devices dd
WHERE dd.promoted_device_id = d.id
  AND (d.chassis_mac IS NULL OR btrim(d.chassis_mac) = '')
  AND dd.remote_chassis_id IS NOT NULL
  AND btrim(dd.remote_chassis_id) <> '';
