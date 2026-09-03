-- Справочник типов узлов: цвет и пользовательские типы.
CREATE TABLE IF NOT EXISTS device_category_defs (
    id          TEXT PRIMARY KEY,
    label       TEXT NOT NULL,
    color       TEXT NOT NULL DEFAULT '#9aa3b5',
    builtin     BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order  INT NOT NULL DEFAULT 100,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT device_category_defs_id_chk CHECK (id ~ '^[a-z][a-z0-9_]{0,31}$'),
    CONSTRAINT device_category_defs_color_chk CHECK (color ~ '^#[0-9A-Fa-f]{6}$')
);

INSERT INTO device_category_defs (id, label, color, builtin, sort_order) VALUES
    ('switch',   'Коммутаторы',     '#4a90e2', TRUE, 10),
    ('router',   'Роутеры',         '#2f9e6f', TRUE, 20),
    ('ap',       'Точки доступа',   '#18c8d6', TRUE, 30),
    ('server',   'Серверы',         '#f0c14a', TRUE, 40),
    ('computer', 'Компьютеры',      '#9b59d0', TRUE, 50),
    ('phone',    'Телефоны',        '#e45c9a', TRUE, 60),
    ('mfu',      'МФУ',             '#8b5a2b', TRUE, 70),
    ('camera',   'Камеры',          '#ff8c00', TRUE, 80),
    ('other',    'Иные устройства', '#c5c9d0', TRUE, 90)
ON CONFLICT (id) DO NOTHING;

-- Снимаем жёсткий CHECK: типы теперь в device_category_defs (+ пользовательские).
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_device_category_check;
