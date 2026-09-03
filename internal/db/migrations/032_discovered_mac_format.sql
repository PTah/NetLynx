-- Привести chassis MAC к aa:bb:cc:dd:ee:ff и унифицировать identity_key chassis:<12hex>.

-- 1) remote_chassis_id: компактный 12-hex → с двоеточиями
UPDATE discovered_devices d
SET remote_chassis_id = lower(
		substr(x.mac, 1, 2) || ':' || substr(x.mac, 3, 2) || ':' || substr(x.mac, 5, 2) || ':' ||
		substr(x.mac, 7, 2) || ':' || substr(x.mac, 9, 2) || ':' || substr(x.mac, 11, 2)
	)
FROM (
	SELECT id,
		lower(regexp_replace(remote_chassis_id, '[^0-9A-Fa-f]', '', 'g')) AS mac
	FROM discovered_devices
	WHERE remote_chassis_id IS NOT NULL
	  AND remote_chassis_id <> ''
) x
WHERE d.id = x.id
  AND length(x.mac) = 12
  AND d.remote_chassis_id !~* '^[0-9a-f]{2}(:[0-9a-f]{2}){5}$';

-- 2) identity_key chassis:… → chassis:<hex без разделителей>
-- Сначала пометить дубликаты (оставляем строку с max(id)).
WITH rewritten AS (
	SELECT id,
		'chassis:' || lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS new_key,
		lower(regexp_replace(substring(identity_key FROM 9), '[^0-9A-Fa-f]', '', 'g')) AS hex
	FROM discovered_devices
	WHERE identity_key ~* '^chassis:'
),
eligible AS (
	SELECT id, new_key
	FROM rewritten
	WHERE length(hex) = 12
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
  AND length(e.hex) = 12
  AND d.identity_key IS DISTINCT FROM e.new_key;
