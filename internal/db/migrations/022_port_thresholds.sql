-- Пороги утилизации на уровне порта (NULL = наследовать с узла / env)

ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS util_high_pct REAL;
ALTER TABLE device_interfaces ADD COLUMN IF NOT EXISTS util_ok_pct REAL;
