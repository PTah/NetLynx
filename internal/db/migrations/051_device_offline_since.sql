-- Момент перехода в оффлайн (для дашборда). NULL = онлайн или момент ещё не известен.

ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS offline_since TIMESTAMPTZ;
