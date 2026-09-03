-- История метрик (CPU, утилизация портов)

CREATE TABLE IF NOT EXISTS metric_samples (
    id          BIGSERIAL PRIMARY KEY,
    device_id   BIGINT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    if_index    INT,
    metric_type TEXT NOT NULL,
    value       REAL NOT NULL,
    sampled_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS metric_samples_device_type_time_idx
    ON metric_samples (device_id, metric_type, sampled_at DESC);

CREATE INDEX IF NOT EXISTS metric_samples_port_time_idx
    ON metric_samples (device_id, if_index, metric_type, sampled_at DESC)
    WHERE if_index IS NOT NULL;
