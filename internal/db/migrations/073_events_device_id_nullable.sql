-- System events (SERVICE_STARTED) are not tied to an inventory device.
ALTER TABLE events ALTER COLUMN device_id DROP NOT NULL;
