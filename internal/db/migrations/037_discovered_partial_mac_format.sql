-- Компактный chassis MAC чётной длины 6–12 hex → aa:bb:… (в т.ч. 10 символов → 01:c0:a8:aa:49).

UPDATE discovered_devices d
SET remote_chassis_id = (
	SELECT lower(string_agg(substr(x.mac, gs, 2), ':' ORDER BY gs))
	FROM generate_series(1, length(x.mac), 2) AS gs
)
FROM (
	SELECT id,
		lower(regexp_replace(remote_chassis_id, '[^0-9A-Fa-f]', '', 'g')) AS mac
	FROM discovered_devices
	WHERE remote_chassis_id IS NOT NULL
	  AND remote_chassis_id <> ''
) x
WHERE d.id = x.id
  AND length(x.mac) BETWEEN 6 AND 12
  AND length(x.mac) % 2 = 0
  AND d.remote_chassis_id !~* ('^[0-9a-f]{2}(:[0-9a-f]{2}){' || ((length(x.mac) / 2) - 1)::text || '}$');

-- identity_key chassis:… → chassis:<hex без разделителей> для чётной длины 6–12
WITH rewritten AS (
	SELECT id,
		'chassis:' || lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS new_key,
		lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS hex
	FROM discovered_devices
	WHERE identity_key ~* '^chassis:'
),
eligible AS (
	SELECT id, new_key, hex
	FROM rewritten
	WHERE length(hex) BETWEEN 6 AND 12
	  AND length(hex) % 2 = 0
),
keepers AS (
	SELECT new_key, max(id) AS keep_id
	FROM eligible
	GROUP BY new_key
),
losers AS (
	SELECT e.id
	FROM eligible e
	JOIN keepers k ON k.new_key = e.new_key
	WHERE e.id <> k.keep_id
)
DELETE FROM discovered_devices d
USING losers l
WHERE d.id = l.id;

UPDATE discovered_devices d
SET identity_key = e.new_key
FROM (
	SELECT id,
		'chassis:' || lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS new_key,
		lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS hex
	FROM discovered_devices
	WHERE identity_key ~* '^chassis:'
) e
WHERE d.id = e.id
  AND length(e.hex) BETWEEN 6 AND 12
  AND length(e.hex) % 2 = 0
  AND d.identity_key IS DISTINCT FROM e.new_key;
