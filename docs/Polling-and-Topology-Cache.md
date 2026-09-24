# Опросы и кэш топологии (blast-graph)

Как NetLynx **собирает** данные о сети и **запоминает** граф связности.

Коротко: отдельного «ночного обхода всей сети» (ping-sweep, скан подсетей, обход неизвестных IP) **нет**. Сеть узнаётся **постоянно** через SNMP-поллер уже известных узлов в inventory. Ночью отдельно пересобирается **кэш графа** для blast/путей из уже накопленных LLDP/CDP/manual рёбер.

См. также: [Autodiscover.md](Autodiscover.md) (SNMP+LLDP на свитчах), [Config-History.md](Config-History.md) (снимки конфигов).

## Слои

### 1. Живой сбор (днём и ночью одинаково)

Планировщик поллера (`POLL_SCHEDULER_SECONDS`, у каждого узла свой `poll_interval_seconds`) опрашивает устройства из списка **Узлы**:

| Что читает | Куда пишет / зачем |
|------------|--------------------|
| LLDP / CDP соседи | `port_neighbors` → кандидаты «Обнаружено»; рёбра карты топологии |
| FDB / MAC | живой снимок FDB, события, линки FDB→топология |
| IF-MIB, PoE, CPU, Printer-MIB… | карточка узла, метрики, графики |

Новые устройства **сами не попадают в Узлы** — только в «Обнаружено». В inventory их добавляют вручную (promote с порта / топологии / списка обнаруженных).

Код: `internal/poller`, запись соседей — `UpsertPortNeighbors` / `SyncDiscoveredFromNeighbors`.

### 2. Ночной (и периодический) пересчёт blast-кэша

Это **не** новый SNMP-обход сети. Job берёт уже известные рёбра из БД и пересобирает сжатый граф для анализа влияния (VLAN delete, shut-impact, L2-path, loops, path A→B).

Реализация: `internal/topologycache` (`Hub.Run`, `Rebuild`).

Триггеры:

| Триггер | Когда |
|---------|--------|
| `startup` | старт `netlynxd` |
| `hourly` | раз в `TOPOLOGY_BLAST_CACHE_INTERVAL_MINUTES` (по умолчанию 60 мин) |
| `night` | wall-clock час `TOPOLOGY_BLAST_NIGHT_HOUR` (по умолчанию **3:00**), один раз в сутки |
| `event:…` | после poller / ручных линков, debounce ~45 с (`NotifyDirty`) |

Шаги `Rebuild`:

1. `BuildTopologyGraphFiltered` — узлы inventory + рёбра из `port_neighbors` / manual (без stale).
2. Выбор root (STP preferred / эвристика).
3. BFS-дистанции, VLAN на рёбрах.
4. Атомарная запись кэша + строка истории.

Таблицы: `topology_blast_meta`, `topology_blast_edges`, `topology_blast_dist`, `topology_blast_edge_vlans`, `topology_blast_history` (хранение ~`TOPOLOGY_BLAST_HISTORY_DAYS`, по умолчанию 30).

Карта в UI («Топология») строится из живых `port_neighbors` и связанных данных; blast-кэш — скелет для расчётов «что отрежется / куда пойдёт путь».

### 3. Другие фоновые задачи «раз в сутки»

| Задача | Расписание | Что помнит |
|--------|------------|------------|
| **FDB daily snapshots** | после успешного FDB-poll, не чаще ~20–24 ч (`FDB_SNAPSHOT_*`) | история «где MAC был N дней назад» |
| **Config snapshots** | интервал ~24 ч (`CONFIG_SNAPSHOT_*`) | `show run` / diff на карточке узла |
| **SSH backup** | wall-clock из UI (час/минута в настройках бэкапа) | архивы конфигов свитчей |

FDB и config — **интервальные**, не привязаны к 3:00. Backup ближе к «ночному», но это бэкап конфигов, не топология.

## Схема потока

```text
SNMP poll (постоянно, inventory)
  → port_neighbors / FDB / metrics / discovered
  → (debounce) NotifyDirty
       ↘
TopologyCache Hub: startup | hourly | night@03:00 | event
  → Rebuild из БД (без SNMP)
  → topology_blast_* + history
```

## Переменные окружения

См. `.env.example`:

| Переменная | Смысл | По умолчанию |
|------------|--------|--------------|
| `TOPOLOGY_BLAST_CACHE_ENABLED` | вкл/выкл кэш | `true` |
| `TOPOLOGY_BLAST_CACHE_INTERVAL_MINUTES` | периодический rebuild | `60` |
| `TOPOLOGY_BLAST_NIGHT_HOUR` | час ночного rebuild (0–23) | `3` |
| `TOPOLOGY_BLAST_HISTORY_DAYS` | срок истории снимков кэша | `30` |
| `FDB_SNAPSHOT_ENABLED` / `_INTERVAL_HOURS` / `_RETENTION_DAYS` | дневные снимки FDB | см. `.env.example` |
| `CONFIG_SNAPSHOT_ENABLED` / `_INTERVAL_HOURS` / `_RETENTION_DAYS` | снимки конфигов | см. `.env.example` |

## Итог

- **Устройства / соседи / MAC** — непрерывный SNMP-poll + БД.
- **«Ночной обход» в коде** — пересборка кэша связности в `TOPOLOGY_BLAST_NIGHT_HOUR` (+ hourly и по событиям), **без** повторного сканирования LAN.
- **Полный ночной discovery неизвестных IP** не реализован.
