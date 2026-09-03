# Postmortem: таймлайн вокруг инцидента

Сводный таймлайн по узлу и LLDP-соседям: события, trap-логи, перемещения MAC, снимки конфига. Нужен, чтобы ответить «что происходило ±N минут вокруг момента».

## Быстрый путь

1. Меню **Postmortem** (`/postmortem`).
2. Выберите узел, момент (`around`), окно (`window`, по умолчанию `5m`), hops соседей (0–3).
3. Смотрите общий таймлайн (сортировка по времени).

## API

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/postmortem?device_id=12&around=2026-09-01T10:00:00Z&window=10m&hops=1"
```

| Параметр | По умолчанию | Смысл |
|----------|--------------|--------|
| `device_id` | обязателен | центр |
| `around` | сейчас | RFC3339 |
| `window` | `5m` | Go duration |
| `hops` | `1` | 0…3 — глубина LLDP-соседей |

Роль: **viewer+**.

## Виды строк таймлайна (`kind`)

| kind | Источник |
|------|----------|
| `event` | таблица `events` |
| `trap` | SNMP trap logs |
| `mac_move` | `mac_fdb_moves` |
| `config_snapshot` | `device_config_snapshots` |

Сейчас **нет** отдельного kind для syslog и **нет** метрик uplink в таймлайне (это backlog в To-Do, не текущий контракт).

## Связанные документы

- [MAC-Investigation.md](MAC-Investigation.md) — разбор конкретного MAC
- [Loop-Investigation.md](Loop-Investigation.md) — циклы LLDP
- [Config-History.md](Config-History.md) — снимки show run
