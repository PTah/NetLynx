# Петли топологии (blast-cache / LLDP)

Отдельный отчёт о **циклах в графе соседей** inventory. Не смешивать с MAC flapping ([MAC-Investigation.md](MAC-Investigation.md)): здесь ищем кольца **устройств**.

С **0.9.0** по умолчанию DFS идёт по **topology blast cache** (тот же скелет, что VLAN blast: LLDP+CDP+manual). Если кэш пуст — fallback на live LLDP.

С **0.10.0** тот же кэш — единый скелет для VLAN delete-impact (глубина `VLAN_BLAST_MAX_DEPTH`, default **32**), MAC L2-path, loops, multi-hop shut-impact и `GET /api/v1/topology/path`.

## Быстрый путь

1. Меню **Петли** (`/investigate/loops`).
2. Кнопка **Обновить** — DFS по blast-cache (или live LLDP).
3. Смотрите список циклов (длина, имена узлов, hops порт→порт).

Параллельные аплинки (два линка между одной парой) тоже попадают в отчёт.

## API

```bash
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/loops"
```

| Параметр | Значение |
|----------|----------|
| `protocol` | пусто / `lldp` — предпочесть blast-cache; `live` / `cdp` — live-граф без кэша |

Роль: **viewer+**.

Ответ: `source` = `blast_cache` \| `live`, `protocol`, `cycles[]` (с `cycle_key`).

## События (0.9.0)

| Тип | Когда |
|-----|--------|
| `L2_LOOP_APPEARED` | Periodic loop-watch (~15 мин) увидел **новый** cycle key (стабильное кольцо не спамит) |
| `PORT_FLAP` | N LINK_UP/DOWN на порту за окно (см. `PORT_FLAP_*` в `.env.example`) |

В MAC-отчёте при гипотезах петли: `loops_touching` + строка `likely_cause` (`l2_loop` / `port_flap_cable` / `stp_reconvergence` / …).

## Что должно получиться

- `cycles[]` — найденные кольца с `hops` (from/to device + ifIndex/ifName).
- `node_count` / `edge_count` — размер графа.
- Пустой список — петли не видно (мало соседей, звезда, или пустой кэш).

## Если пусто, а петля «есть»

| Симптом | Что проверить |
|---------|----------------|
| Петля через неуправляемый свитч | LLDP её не увидит — смотрите [MAC-Investigation.md](MAC-Investigation.md) |
| Нет соседей на карте | LLDP на портах; [Autodiscover.md](Autodiscover.md); rebuild blast-cache |
| Нужен разбор по времени | [Postmortem.md](Postmortem.md) вокруг момента инцидента |
