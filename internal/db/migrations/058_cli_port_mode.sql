-- CLI switchport mode from show running-config (SSH); poller must not overwrite port_role when set.

ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS cli_port_mode TEXT;
ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS cli_access_vlan INT;
ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS cli_mode_synced_at TIMESTAMPTZ;
