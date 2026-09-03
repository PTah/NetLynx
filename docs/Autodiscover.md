# Autodiscover устройств на портах (LLDP / CDP)

## Реализовано

При каждом успешном опросе IF-MIB дополнительно читаются LLDP и CDP; соседи пишутся в `port_neighbors`. Отдельно FDB может создавать соседей `protocol=fdb` (AP/edge-эвристики) для карты топологии.

### LLDP

- `lldpRemSysName`, `lldpRemPortId`, `lldpRemChassisId`
- subtype chassis (в т.ч. networkAddress → `remote_mgmt_addr`)
- сопоставление local port → ifIndex через `lldpLocPortTable`, иначе ifIndex / ifName `0/N`

### CDP (Cisco)

- `cdpCacheDeviceId` / `cdpCacheDevicePort` / address
- индекс `ifIndex.deviceIndex`

### Хранение

Таблица `port_neighbors`:

- PK `(device_id, if_index, protocol, rem_index)`
- `protocol`: `lldp` | `cdp` | `fdb`
- `stale` + TTL ~**2 ч**
- При **ошибке** walk данные **не очищаются**
- Пустой успешный walk: соседи помечаются stale только после **двух подряд** пустых опросов (`neighborEmptyConfirmPolls = 2`), чтобы не мигать при одноразовом сбое LLDP

## API / UI

| Что | Где |
|-----|-----|
| Соседи на портах | `GET /devices/{id}/detail` → `neighbors`; колонка **«Сосед LLDP»** |
| Кандидаты в inventory | страница **Обнаружено** (`/discovered`): ignore / reopen / preview / promote |
| Карта | **Топология** (`/topology`): фильтры protocol (lldp/cdp/fdb/manual), VLAN, локация, depth, layout |
| Настройки карты | `GET/PATCH /api/v1/settings/topology` |

Ручные связи — [Manual-Topology-Links.md](Manual-Topology-Links.md).

## Резолв в известный узел

1. **MAC** — chassis / MAC в Port ID  
2. **mgmt IP** — `remote_mgmt_addr` / host  
3. **sysName** — только если уникален среди узлов  

При poll пишется `devices.chassis_mac`; при promote MAC из discovered сразу в `chassis_mac`.

### Ловушка EdgeSwitch ifName `0/10+`

OctetString `0/24` нельзя трактовать как IPv4 (`48.47.50.52`).

## Статус фич

| Фича | Статус |
|------|--------|
| discovered_devices + promote/ignore | **готово** (исторически 0.16) |
| FDB → topology neighbors | **готово** (`protocol=fdb`) |
| Фильтры/layout топологии | **готово** в UI |

## Диагностика

- LLDP/CDP включены на портах?
- `snmpwalk` LLDP rem sysName / CDP deviceId
- «Показывать все интерфейсы» — сосед может быть на нефизическом ifIndex
- Логи: `lldp neighbors walk` / `cdp …` / `… write`
