# Ручные линки топологии

Когда LLDP/CDP ещё не видит линк или **врёт** (например, несколько VLAN/соседей на одном SFP MikroTik), можно задать связь **порт ↔ порт** вручную. Она попадает на карту **Топологии** и в карточку узла.

## Модель

Таблица `manual_topology_links` (отдельно от `port_neighbors`):

- концы: `a_device_id`/`a_if_index` и `b_device_id`/`b_if_index` (оба порта обязательны);
- пара хранится в каноническом порядке (`a_device_id < b_device_id`);
- `status`: `active` | `superseded`;
- при supersede: `superseded_at`, `superseded_by` (`lldp` / `cdp`) — только вручную / админ-инструментами (не из poller).

Ручные строки **не** пишутся в `port_neighbors`: poller помечает невиденное `stale` и чистит по TTL.

## Приоритет над LLDP/CDP

Пока связь **`active`**, она **закреплена**:

1. Poller **не** переводит её в `superseded` при появлении LLDP/CDP.
2. На карте топологии LLDP/CDP-рёбра с локальных портов обоих концов ручной связи **не показываются** — остаётся ручной линк.

Снять закрепление: удалить ручную запись (или пометить superseded вручную через API, если нужно).

## Сборка графа

`BuildTopologyGraph` добавляет `active` ручные рёбра с `protocol: "manual"` и полем `manual_link_id`. Фильтр `GET /api/v1/topology?protocol=manual` поддерживается.

## API

| Method | Path | Назначение |
|--------|------|------------|
| GET | `/api/v1/manual-links?device_id=&status=&limit=` | список (`status` пустой или `all` — все; иначе фильтр) |
| POST | `/api/v1/manual-links` | создать |
| PATCH | `/api/v1/manual-links/{id}` | порты / note / restore |
| DELETE | `/api/v1/manual-links/{id}` | удалить |

Тело создания: `{a_device_id, a_if_index, b_device_id, b_if_index, note?}`.

## UI

- **Топология**: режим «Связать», фиолетовый пунктир для pure-manual, инспектор (заметка / удалить).
- **Карточка узла**: секция «Ручные связи» — таблица, добавить / изменить / восстановить / удалить.

См. также [Autodiscover.md](Autodiscover.md).
