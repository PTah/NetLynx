# Петли топологии (LLDP)

Отдельный отчёт о **циклах в графе соседей** inventory. Не смешивать с MAC flapping ([MAC-Investigation.md](MAC-Investigation.md)): здесь ищем кольца **устройств**, видимые по LLDP (и опционально CDP в API).

## Быстрый путь

1. Меню **Петли** (`/investigate/loops`).
2. Кнопка **Обновить** — DFS по текущим LLDP-соседям.
3. Смотрите список циклов (длина, имена узлов, hops порт→порт).

UI всегда запрашивает `protocol=lldp`. Параллельные аплинки (два линка между одной парой) тоже попадают в отчёт.

## API

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/loops?protocol=lldp"
```

| Параметр | Значение |
|----------|----------|
| `protocol` | `lldp` (UI); пусто = lldp+cdp resolved; `cdp` — только CDP |

Роль: **viewer+**.

## Что должно получиться

- `cycles[]` — найденные кольца с `hops` (from/to device + ifIndex/ifName).
- `node_count` / `edge_count` — размер графа.
- Пустой список — по LLDP петли не видно (мало соседей или звезда без цикла).

## Если пусто, а петля «есть»

| Симптом | Что проверить |
|---------|----------------|
| Петля через неуправляемый свитч | LLDP её не увидит — смотрите [MAC-Investigation.md](MAC-Investigation.md) |
| Нет соседей на карте | LLDP на портах; [Autodiscover.md](Autodiscover.md) |
| Нужен разбор по времени | [Postmortem.md](Postmortem.md) вокруг момента инцидента |
