-- FDB learning/baseline timestamp per device.

ALTER TABLE devices ADD COLUMN IF NOT EXISTS fdb_baseline_at TIMESTAMPTZ;
