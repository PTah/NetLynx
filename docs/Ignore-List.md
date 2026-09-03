# Ignore list — отключение реакции на порту

## Назначение

Для пары **узел + ifIndex** задать правила, при которых NetLynx **не реагирует** на события так, как обычно: не шлёт уведомления, не выполняет incident actions, опционально — не создаёт запись в `events`.

Типичные случаи:

- Uplink / trunk с легитимной сменой MAC.
- Порт с известным «шумным» оборудованием.
- Временное обслуживание (комментарий в `comment`).

## Модель (`port_event_ignore`)

| Поле | По умолчанию | Смысл |
|------|--------------|--------|
| `event_types` | NULL / `*` | Список типов через запятую; пусто = все типы |
| `block_events` | false | Не писать в журнал событий |
| `block_notify` | true | Не webhook / Telegram / email |
| `block_actions` | true | Не SNMP admin down и прочие действия |
| `comment` | — | Пояснение для админа |

## API

- `GET /api/v1/devices/{id}/port-ignores` — список правил.
- `PUT /api/v1/devices/{id}/interfaces/{ifIndex}/ignore` — создать/обновить.
- `DELETE /api/v1/devices/{id}/interfaces/{ifIndex}/ignore` — снять.

### Режимы из UI (цикл по кнопке)

| mode | Кнопка | Поведение |
|------|--------|-----------|
| `off` | белая **Монит.** | правило удаляется |
| `soft` | жёлтая **Тихий** | без notify/actions для link, util, speed, `MAC_REMOVED`, `ACCESS_PORT_LONG_IDLE_DEVICE`, `ACCESS_PORT_MAC_SUBSTITUTED`; **UNKNOWN_MAC**, **MAC_MOVED**, **MAC_FLAPPING**, **MAC_MULTI_ACCESS** — как обычно; события в журнале |
| `all` | красная **Выкл** | `block_events` + notify + actions для перечисленных типов правила; **исключение:** часть эмитов (`MAC_MULTI_ACCESS`, syslog `MAC_FLAPPING`) идут с `ignore=nil` и **не** смотрят port-ignore |

Тело `PUT` с пресетом:

```json
{ "mode": "soft" }
```

Тело `PUT` вручную (как раньше):

```json
{
  "event_types": "UNKNOWN_MAC_ON_ACCESS_PORT,MAC_MOVED",
  "block_events": false,
  "block_notify": true,
  "block_actions": true,
  "comment": "Uplink к ядру"
}
```

В detail у интерфейса: `ignore_mode` — `off` | `soft` | `all`.

## Поведение в поллере

При `emit()`:

1. Если есть правило для `if_index` и тип события совпадает с `event_types`:
   - `block_events` → событие не создаётся.
   - иначе событие в БД, но без notify/actions при соответствующих флагах.

## UI

На карточке узла в таблице портов колонка **Монит.** — цикл:

| Кнопка | mode | Когда использовать |
|--------|------|--------------------|
| **Монит.** | `off` | обычный мониторинг |
| **Тихий** (жёлтая) | `soft` | без Telegram по link/util/speed/idle/substituted; UNKNOWN_MAC / MAC_MOVED / FLAPPING / MULTI_ACCESS остаются |
| **Выкл** (красная) | `all` | почти полный игнор порта; см. исключение syslog/MULTI_ACCESS выше |

Поиск на порту (описание / MAC / IP): `GET /api/v1/ports/search?q=...` (раздел «Узлы» и поиск на «Топологии»).  
MAC/IP: сначала порты с **LLDP** на этот адрес (физический сосед), затем записи **FDB** (MAC может быть виден на uplink’ах всего VLAN — это не «подключён ко всем свитчам»).

В API detail: `port_ignores[]`, у интерфейса флаг `event_ignored`.

## Связь с п. 6 (инциденты)

Ignore list — **обязательный предохранитель** перед расширением auto-actions: сначала ignore на uplink, потом включать `incident_action_enabled` только на access-портах.
