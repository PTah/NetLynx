-- Port role for intrusion heuristics (step 14)

ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS port_role TEXT NOT NULL DEFAULT 'auto';
