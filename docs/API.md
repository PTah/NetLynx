# HTTP API NetLynx

NetLynx отдаёт API **сам** — процесс `netlynxd` слушает порт из настроек (по умолчанию **8080**).

**Базовый URL** (типичная установка без nginx):

```text
http://<IP_или_имя_сервера>:8080
```

Примеры:

- `http://10.0.0.1:8080/api/v1/devices`
- `http://127.0.0.1:8080/health` — с самого сервера

**Если вы поставили nginx** ([install-nginx.sh](install-nginx.sh)) — снаружи 80/443, nginx → `127.0.0.1:8080`. Reverse proxy **не обязателен**.

Аутентификация: **Bearer** `access_token` в `Authorization`, либо **Basic** с сервера (`curl -u`). Refresh — cookie `HttpOnly` после `POST /api/v1/auth/login`.

## Роли (RBAC)

| Роль | Доступ |
|------|--------|
| **viewer** | чтение: узлы, порты, события, топология, discovered, MAC/петли/postmortem, метрики |
| **operator** | + изменение узлов, портов, уведомлений (в т.ч. incident actions), discovered, manual-links, snapshots |
| **admin** | + пользователи, аудит, бэкапы, журнал службы, destructive inventory |

Подробнее: [Roles.md](Roles.md).

---

## Без аутентификации

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health` | `{ "status": "ok", "version", … }` |

---

## Auth — `/api/v1/auth`

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| POST | `/login` | — | `{ username, password }` → `access_token`, refresh-cookie |
| POST | `/refresh` | cookie | новый `access_token` |
| POST | `/logout` | cookie | завершить сессию |
| GET | `/me` | auth | текущий пользователь |
| POST | `/sse-ticket` | auth | ticket для SSE (EventSource без Bearer) |

---

## Устройства — viewer+

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/devices` | список узлов |
| GET | `/api/v1/devices/{id}` | один узел |
| GET | `/api/v1/devices/{id}/detail` | узел + порты + события + neighbors |
| GET | `/api/v1/devices/{id}/interfaces` | порты |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/clients` | MAC/IP (FDB+ARP) |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/settings` | ignore, thresholds |
| GET | `/api/v1/devices/{id}/interfaces/{ifIndex}/shut-impact` | превью риска shutdown |
| GET | `/api/v1/devices/{id}/events` | события узла |
| GET | `/api/v1/devices/{id}/metrics` | история метрик |
| GET | `/api/v1/devices/{id}/traffic-series` | трафик по портам |
| GET | `/api/v1/devices/{id}/config/snapshots` | снимки show run |
| GET | `/api/v1/devices/{id}/config/snapshots/{snapId}` | один снимок |
| GET | `/api/v1/devices/{id}/config/diff?from=&to=` | diff (можно опустить — последний) |
| GET | `/api/v1/devices/{id}/vlans` | VLAN database + членство портов (show run / порты); FDB только для уже известных VLAN (не создаёт строки-призраки) |
| GET | `/api/v1/devices/{id}/fdb/snapshots` | ежедневные снимки FDB |
| GET | `/api/v1/ports/search?q=` | поиск порта |

### CRUD и диагностика — operator+

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/v1/devices` | создать |
| PATCH | `/api/v1/devices/{id}` | SNMP-параметры |
| PATCH | `/api/v1/devices/{id}/name` | имя |
| PATCH | `/api/v1/devices/{id}/host` | IP/DNS |
| PATCH | `/api/v1/devices/{id}/chassis-mac` | chassis MAC |
| PATCH | `/api/v1/devices/{id}/location` | расположение |
| PATCH | `/api/v1/devices/{id}/category` | тип |
| PATCH | `/api/v1/devices/{id}/poll-interval` | интервал |
| PATCH | `/api/v1/devices/{id}/monitoring` | пороги FDB/CPU |
| PATCH | `/api/v1/devices/{id}/online-override` | ручной online |
| PATCH | `/api/v1/devices/{id}/trust-link-traps` | флаг для link traps |
| PATCH | `/api/v1/devices/{id}/ssh` | SSH узла |
| DELETE | `/api/v1/devices/{id}` | удалить один |
| POST | `/api/v1/devices/{id}/snmp-test` | SNMP test |
| POST | `/api/v1/devices/{id}/traceroute` | traceroute с сервера |
| POST | `/api/v1/devices/{id}/tcp-probe` | TCP connect к порту |
| POST | `/api/v1/devices/{id}/sync-port-roles-from-config` | роли из show run |
| POST | `/api/v1/devices/{id}/config/snapshot` | снимок сейчас |
| DELETE | `/api/v1/devices` | admin — все узлы; заголовок `X-Confirm: DELETE-ALL-DEVICES` |

### Порты — operator+

| Метод | Путь | Описание |
|-------|------|----------|
| PATCH | `.../interfaces/{ifIndex}/descr` | подпись |
| PATCH | `.../interfaces/{ifIndex}/admin` | admin up/down |
| PATCH | `.../interfaces/{ifIndex}/poe` | PoE |
| PATCH | `.../interfaces/{ifIndex}/isolate` | изоляция |
| PATCH | `.../interfaces/{ifIndex}/dhcp-snooping` | DHCP snooping |
| PATCH | `.../interfaces/{ifIndex}/flow-control` | flow control |
| PATCH | `.../interfaces/{ifIndex}/stp` | STP |
| PATCH | `.../interfaces/{ifIndex}/vlan` | `{ op, vlan_id?, allowed_mode?, allowed_vlans? }` — `set_access` / `remove` / `trunk_allow` (`add`\|`remove`\|`all`\|`except`); legacy `add_tagged`; ответ: `vlans[]`. UI предупреждает, если в allowed нет VLAN 1 (риск потери SSH/Web) |
| POST | `/api/v1/devices/{id}/vlans` | `{ "vlan_id": N, "name"? }` — создать в vlan database; ответ: `vlans[]` + `source` |
| DELETE | `/api/v1/devices/{id}/vlans` | `{ "vlan_ids": [167,30,31] }` — массовое удаление (`no vlan …`); **409**, если VLAN ещё на портах (access/tagged) |
| PATCH | `/api/v1/devices/{id}/vlans/{vlanId}` | `{ "name": "..." }` — имя в vlan database; ответ: `vlans[]` + `source` для UI |
| DELETE | `/api/v1/devices/{id}/vlans/{vlanId}` | удалить один VLAN; **409**, если прописан на портах; ответ: `vlans[]` + `source` |
| PATCH | `.../interfaces/{ifIndex}/thresholds` | util |
| PUT/DELETE | `.../interfaces/{ifIndex}/ignore` | ignore |
| GET | `/api/v1/devices/{id}/port-ignores` | список (operator); на detail viewer видит флаги |
| POST | `.../clients/preview` | SNMP перед promote |
| POST | `.../clients/promote` | узел с FDB-порта |

---

## Расследование — viewer+ / operator+

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/api/v1/investigate/mac?mac=` | viewer | отчёт MAC |
| GET | `/api/v1/investigate/mac/flappers` | viewer | топ «горячих» MAC |
| GET | `/api/v1/investigate/mac/fdb-history` | viewer | история по снимкам FDB |
| PATCH | `/api/v1/investigate/mac/status` | operator | open/resolved/ignored |
| GET | `/api/v1/investigate/loops?protocol=` | viewer | петли LLDP/CDP |
| GET | `/api/v1/postmortem?device_id=&around=&window=&hops=` | viewer | таймлайн инцидента |

См. [MAC-Investigation.md](MAC-Investigation.md), [Loop-Investigation.md](Loop-Investigation.md), [Postmortem.md](Postmortem.md).

---

## События и live

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/api/v1/events` | viewer | лента |
| GET | `/api/v1/events/stream?ticket=` | ticket/Bearer | SSE |

---

## Топология и discovered

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/topology` | граф LLDP/CDP/FDB/manual |
| GET/PATCH | `/api/v1/settings/topology` | настройки карты (viewer get / operator patch) |
| GET | `/api/v1/discovered` | кандидаты LLDP/CDP |
| POST | `/api/v1/discovered/{id}/preview\|promote\|ignore\|reopen` | operator |
| GET | `/api/v1/manual-links` | viewer |
| POST/PATCH/DELETE | `/api/v1/manual-links[/{id}]` | operator |

---

## Настройки

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET/PATCH | `/api/v1/settings/notifications` | operator | webhook, TG, email, **incident_action_*** (enabled, event_types, dry_run, cooldown_seconds) |
| POST | `/api/v1/settings/notifications/email-test` | operator | тест SMTP |
| GET/PATCH | `/api/v1/settings/uisp` | operator | UISP |
| GET | `/api/v1/settings/device-categories` | viewer | типы |
| POST/PATCH/DELETE | `/api/v1/settings/device-categories[/{id}]` | operator | CRUD типов |
| GET/PATCH | `/api/v1/settings/mac-investigation` | get viewer / patch operator | WiFi OUI и пр. |
| GET | `/api/v1/settings/inventory/stale-fdb` | viewer | превью stale live FDB |
| POST | `/api/v1/settings/inventory/stale-fdb/clear` | operator | `X-Confirm: CLEAR-STALE-FDB` |
| GET | `/api/v1/settings/inventory/offline-devices` | viewer | оффлайн узлы |
| POST | `/api/v1/settings/inventory/offline-devices/delete` | admin | `X-Confirm: DELETE-OFFLINE-DEVICES` |
| POST | `/api/v1/devices/import-uisp` | operator | импорт UISP |

### Резервные копии — admin

| Метод | Путь | Описание |
|-------|------|----------|
| GET/PATCH | `/api/v1/settings/backup` | расписание, SMB, retention |
| POST | `/api/v1/backup/run` | запуск |
| GET | `/api/v1/backup/archives` | список |
| POST | `/api/v1/backup/verify` | проверка ZIP |
| POST | `/api/v1/backup/import` | импорт на чистую БД |

### Журнал службы — admin

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/settings/journal` | метаданные |
| GET | `/api/v1/settings/journal/lines` | строки |
| GET | `/api/v1/settings/journal/stream?ticket=` | SSE |

### SNMP traps — operator+

| Метод | Путь | Описание |
|-------|------|----------|
| GET/PATCH | `/api/v1/settings/snmp-traps` | listener, log, include labels, `link_trap_events_mode` (`off`/`per_device`/`all`), `link_trap_effects` |
| GET/DELETE | `/api/v1/settings/snmp-traps/logs` | журнал / очистка |

---

## Пользователи и аудит — admin

| Метод | Путь | Описание |
|-------|------|----------|
| GET/POST/PATCH/DELETE | `/api/v1/users[/{id}]` | CRUD |
| GET | `/api/v1/audit` | журнал |

---

## Система — viewer+

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/system/stats` | CPU/RAM/disk хоста |
| GET | `/metrics` | Prometheus (Bearer viewer+) |

---

## Destructive confirm

Не `X-Confirm: 1`, а **точный токен**:

| Операция | Заголовок |
|----------|-----------|
| Удалить все узлы | `X-Confirm: DELETE-ALL-DEVICES` |
| Очистить stale live FDB | `X-Confirm: CLEAR-STALE-FDB` |
| Удалить оффлайн-узлы | `X-Confirm: DELETE-OFFLINE-DEVICES` |

---

## Планировщик SNMP (env)

- `POLL_SCHEDULER_SECONDS` — тик планировщика (по умолчанию 10 с).
- `poll_interval_seconds` на узле — интервал IF-MIB.
- `FDB_POLL_INTERVAL_SECONDS` — опрос FDB.

См. [`.env.example`](../.env.example).

## Ошибки

`{ "error": "текст" }`, HTTP 4xx/5xx.
