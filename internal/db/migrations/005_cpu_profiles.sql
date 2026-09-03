-- CPU профиль и последнее значение CPU по узлу

ALTER TABLE devices ADD COLUMN IF NOT EXISTS cpu_profile TEXT;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_cpu_pct REAL;
ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_cpu_at TIMESTAMPTZ;
