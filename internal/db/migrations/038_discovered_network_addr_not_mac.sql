-- Ложный «MAC» из LLDP networkAddress (01 + IPv4) → remote_mgmt_addr, очистить chassis.

UPDATE discovered_devices
SET
	remote_mgmt_addr = COALESCE(
		NULLIF(trim(remote_mgmt_addr), ''),
		(
			(('x' || substr(h.hex, 3, 2))::bit(8)::int)::text || '.' ||
			(('x' || substr(h.hex, 5, 2))::bit(8)::int)::text || '.' ||
			(('x' || substr(h.hex, 7, 2))::bit(8)::int)::text || '.' ||
			(('x' || substr(h.hex, 9, 2))::bit(8)::int)::text
		)
	),
	remote_chassis_id = NULL
FROM (
	SELECT id,
		lower(regexp_replace(COALESCE(remote_chassis_id, ''), '[^0-9A-Fa-f]', '', 'g')) AS hex
	FROM discovered_devices
	WHERE remote_chassis_id IS NOT NULL
	  AND remote_chassis_id <> ''
) h
WHERE discovered_devices.id = h.id
  AND length(h.hex) = 10
  AND h.hex LIKE '01%';
