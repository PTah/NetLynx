# Расследование MAC-аномалий

Модуль помогает разобрать **MAC flapping**, блуждающие адреса и ситуации «один MAC на нескольких access-портах», когда топология LLDP/CDP не показывает кольцо.

См. также: [Loop-Investigation.md](Loop-Investigation.md) (петли устройств), [Postmortem.md](Postmortem.md) (таймлайн вокруг момента).

## Быстрый путь

1. Меню **MAC** (`/investigate/mac`).
2. **Горячие MAC** или ввод адреса вручную.
3. Смотрите: баннер multi-access (если есть), **L2-путь**, **карту перемещений**, footprint, **гипотезы**, timeline, **FDB history**.
4. Статус: **открыто** / **закрыто** / **игнор** (operator).
5. При необходимости — превью **shut-impact** и admin-down порта; WiFi-MAC без трекинга → баннер и Настройки → MAC.

Либо «расследовать» у события `MAC_FLAPPING` / `MAC_MOVED` / `MAC_MULTI_ACCESS`, либо клик по MAC в поиске портов.

## API

```bash
# отчёт
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac?mac=52:54:4c:83:09:e0"

# горячие MAC (UI: hours=24, min_moves=2 — не путать с порогом события MAC_FLAP_MIN_MOVES)
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac/flappers?hours=24&min_moves=2"

# история по снимкам FDB
curl -sS -H "Authorization: Bearer $TOKEN" \
  "http://127.0.0.1:8080/api/v1/investigate/mac/fdb-history?mac=52:54:4c:83:09:e0"
```

| Метод | Путь | Роль |
|-------|------|------|
| GET | `/investigate/mac` | viewer |
| GET | `/investigate/mac/flappers` | viewer |
| GET | `/investigate/mac/fdb-history` | viewer |
| PATCH | `/investigate/mac/status` | operator |
| GET/PATCH | `/settings/mac-investigation` | get viewer / patch operator |

WiFi-MAC из списка «не трекать» → **404** `ErrWiFiMACNotTracked`.

## Гипотезы (примеры)

`unmanaged_loop`, `kvm_dual_uplink`, `core_loop_broadcast`, `misclassified_uplink`, `virtualization_mac`, `dual_homed_or_clone`, `ap_roaming`, `rogue_mac`, `duplicate_mac`, `port_storm`, `insufficient_data`, …

`kvm_dual_uplink` может глушиться при сильном `core_loop_broadcast`.

## Источники данных

| Источник | Как | Зачем |
|----------|-----|--------|
| FDB poll + `mac_fdb_moves` | всегда | медленные перемещения |
| Daily `fdb_snapshots` | `FDB_SNAPSHOT_*` | «где был N дней назад» |
| Syslog UDP | `NETLYNX_SYSLOG_LISTEN=:9514` | realtime flap: Eltex `%BRG_MACNTFY…`, Cisco `%SW_MATM-4-MACFLAP_NOTIF` / `MACFLAP` |

Без syslog секундный flapping часто не попадает в FDB-снимок.

### Syslog (пример Eltex)

```bash
# /etc/netlynx/netlynx.env
NETLYNX_SYSLOG_LISTEN=:9514
```

На свитче — UDP на IP NetLynx. Сообщение flapping → `MAC_FLAPPING` + история.

## Пороги (env)

| Переменная | Default | Смысл |
|------------|---------|--------|
| `MAC_FLAP_MIN_MOVES` | 3 | смен порта за окно → событие `MAC_FLAPPING` |
| `MAC_FLAP_WINDOW_SECONDS` | 3600 | окно |
| `MAC_FLAP_DEBOUNCE_SECONDS` | 900 | антиспам |
| `MAC_MOVES_RETENTION_DAYS` | 14 | хранение moves |
| `NETLYNX_SYSLOG_LISTEN` | пусто | UDP; `off` = выкл |
| `FDB_SNAPSHOT_ENABLED` | true | ежедневные снимки |
| `FDB_SNAPSHOT_INTERVAL_HOURS` | 24 | |
| `FDB_SNAPSHOT_RETENTION_DAYS` | 30 | |
| `FDB_STALE_CLEAR_DAYS` | 60 | авто-очистка live FDB (0 = выкл) |

## Если не работает

| Симптом | Что сделать |
|---------|-------------|
| Timeline пустой | 2+ опросов FDB; статус FDB на узле |
| Syslog не ловится | firewall UDP; host = `devices.host`; имена портов ≈ `if_name` |
| Ложный flapping на uplink | роль trunk / sync из SSH |
| Нет кольца на топологии | нормально для unmanaged / KVM dual-uplink — смотрите гипотезы |
| MAC `52:54:…` между двумя access | `kvm_dual_uplink`: проверка admin-down одного порта; STP/bond на гипервизоре |
