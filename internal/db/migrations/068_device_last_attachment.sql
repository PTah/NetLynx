-- Последняя известная привязка ПК/камеры к порту свитча (для оповещений DEVICE_ONLINE до FDB poll).
ALTER TABLE devices
  ADD COLUMN IF NOT EXISTS attach_parent_id BIGINT REFERENCES devices(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS attach_if_index INT,
  ADD COLUMN IF NOT EXISTS attach_protocol TEXT,
  ADD COLUMN IF NOT EXISTS attach_updated_at TIMESTAMPTZ;

-- Backfill из живых port_neighbors (access FDB / LLDP / manual), не trunk ghost.
UPDATE devices d
SET attach_parent_id = pn.device_id,
    attach_if_index = pn.if_index,
    attach_protocol = pn.protocol,
    attach_updated_at = pn.last_seen_at
FROM port_neighbors pn
JOIN device_interfaces di ON di.device_id = pn.device_id AND di.if_index = pn.if_index
WHERE d.chassis_mac IS NOT NULL AND btrim(d.chassis_mac) <> ''
  AND d.device_category NOT IN ('switch', 'router')
  AND pn.stale = false
  AND pn.remote_chassis_id IS NOT NULL
  AND lower(replace(replace(pn.remote_chassis_id, ':', ''), '-', ''))
      = lower(replace(replace(d.chassis_mac, ':', ''), '-', ''))
  AND (
    pn.protocol IN ('manual', 'lldp', 'cdp')
    OR (pn.protocol = 'fdb' AND COALESCE(di.port_role, '') = 'access')
  )
  AND pn.last_seen_at > now() - interval '7 days';
