-- Индекс для сводок по типу события за период (Настройки → уведомления → Статистика).
CREATE INDEX IF NOT EXISTS events_type_created_idx ON events (event_type, created_at DESC);
