-- Цвета как на топологии (TOPOLOGY_DOT) + флаг мигания точки.
ALTER TABLE device_category_defs
    ADD COLUMN IF NOT EXISTS blink BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE device_category_defs SET color = v.color, blink = v.blink, updated_at = now()
FROM (VALUES
    ('switch',   '#4a90e2', FALSE),
    ('router',   '#2f9e6f', FALSE),
    ('ap',       '#18c8d6', FALSE),
    ('server',   '#f0c14a', FALSE),
    ('computer', '#9b59d0', FALSE),
    ('phone',    '#e45c9a', FALSE),
    ('mfu',      '#8b5a2b', FALSE),
    ('camera',   '#ff8c00', TRUE),
    ('other',    '#c5c9d0', FALSE)
) AS v(id, color, blink)
WHERE device_category_defs.id = v.id AND device_category_defs.builtin = TRUE;
