-- Кандидаты со статусом added без живого узла снова становятся new.
UPDATE discovered_devices
SET status = 'new',
    promoted_device_id = NULL,
    updated_at = now()
WHERE status = 'added'
  AND (
    promoted_device_id IS NULL
    OR NOT EXISTS (SELECT 1 FROM devices d WHERE d.id = discovered_devices.promoted_device_id)
  );
