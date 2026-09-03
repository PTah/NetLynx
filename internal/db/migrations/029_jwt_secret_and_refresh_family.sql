-- JWT secret в БД + refresh-token family для reuse detection

CREATE TABLE IF NOT EXISTS app_secrets (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE auth_sessions
    ADD COLUMN IF NOT EXISTS family_id UUID,
    ADD COLUMN IF NOT EXISTS rotated_from BIGINT REFERENCES auth_sessions (id) ON DELETE SET NULL;

-- Существующие сессии: каждая — своя семья (старые refresh не объединяем).
UPDATE auth_sessions
SET family_id = gen_random_uuid()
WHERE family_id IS NULL;

ALTER TABLE auth_sessions
    ALTER COLUMN family_id SET NOT NULL,
    ALTER COLUMN family_id SET DEFAULT gen_random_uuid();

CREATE INDEX IF NOT EXISTS auth_sessions_family_idx ON auth_sessions (family_id);
