# NetLynx To-Do (живой план)

Этот файл — основной план работ. Здесь отмечаем выполненные шаги и добавляем новые пожелания.

## Как читать этот файл

- **`[x]`** — сделано и в проде (или в коде).
- **`[ ]` в разделах «Отложено»** — **не пропустили**: сознательный backlog на refactor/polish/desktop. См. [Roadmap.md](Roadmap.md).
- Основной чеклист (шаги 1–16) **закрыт**; шаг 17 (desktop) — **черновик, не продукт**. Актуальная сверка фич — [TZ §12](TZ-snmp-switch-monitor.md#12-статус-реализации-актуально-на-05x) и [Roadmap.md](Roadmap.md) (**0.6.0**).

---

Источник: пункт 10 из `docs/TZ-snmp-switch-monitor.md`.

## Порядок разработки (чеклист)

- [x] 1. Репозиторий и каркас: структура модулей, конфиг, логирование.
- [x] 2. БД и миграции: узлы, интерфейсы, события, хранение состояния.
- [x] 3. SNMP-база: v2c/v3, тестовый вызов `sysName/sysDescr`.
- [x] 4. Планировщик поллера: очередь задач по узлам и интервалам, ограничение параллелизма.
- [x] 5. IF-MIB: опрос интерфейсов и события `Link UP / Link DOWN`.
- [x] 6. Утилизация порта: `ifHCInOctets/ifHCOutOctets`, пороги, гистерезис, события `PORT_UTILIZATION_HIGH/OK`.
- [x] 7. HTTP API (read-first): узлы, детали, порты, лента событий, фильтры.
- [x] 8. Аутентификация API и веба: реализованы backend JWT/refresh, login/logout/me и веб-вход с Bearer + auto-refresh (Basic оставлен как совместимость на переходный период).
- [x] 9. Веб-оболочка: меню, маршрутизация, layout.
- [x] 10. Дашборд: узлы и события есть, CPU отображается в блоке коммутаторов.
- [x] 11. Профили вендора + CPU: Ubiquiti, MikroTik, SNR и расширенный набор (Cisco/Huawei/…, см. Vendors) + вывод в UI.
- [x] 12. SNMP v2c/v3 — доведение: валидация v3, edge-cases.
- [x] 13. FDB / MAC по портам: подпланировщик, BRIDGE-MIB + **Q-BRIDGE**, снапшот в БД.
- [x] 14. Эвристика интруза: `UNKNOWN_MAC_ON_ACCESS_PORT` + `MAC_MOVED`, trunk/ignore, baseline learning.
- [x] 15. Уведомления: webhook + Telegram + Email, подписки, retry/backoff.
- [x] 16. Упаковка Linux/Windows: deploy/runbook + Windows Server setup.
- [ ] 17. Windows desktop-клиент: **черновик** Electron (логин/список); **не** production; токен в `localStorage`, не safeStorage.

## Добавленные приоритеты (текущий фокус)

- [x] Шаг 11 (CPU профили): закрыт для Ubiquiti, MikroTik, SNR + вывод в дашборде/карточке узла.
- [x] Шаг 12 (SNMP v3 hardening): расширена валидация v3 и закрыты базовые edge-cases.
- [x] Шаг 15 (уведомления до production): email + подписки по типам + retry/backoff реализованы.
- [x] Шаги 13-14 (FDB/MAC + интруз): шаг 13 закрыт; шаг 14 закрыт (access/trunk/ignore эвристики + baseline learning).
- [x] Шаг 16 (упаковка/эксплуатация): финальные systemd/deploy/runbook + Windows Server doc.

## Расширения (0.12.0)

- [x] История метрик + графики на карточке узла.
- [x] RBAC (admin/operator/viewer) + журнал аудита.
- [x] Unit-тесты poller/snmp + CI (GitHub Actions).
- [x] SSE live-обновления событий.
- [x] Опциональные действия при инцидентах (admin down порта).
- [x] CPU-профили Cisco/Juniper/ELTEX + статус FDB на узле.
- [x] Desktop: Settings (черновик); **safeStorage — нет** (токен в localStorage) — см. backlog.

## Расширения (0.13.0)

- [x] п.7: FDB interval per-device + статус на узле.
- [x] п.8: пороги утилизации per-device / per-port.
- [x] Ignore list (порт + типы событий).
- [x] Autodiscover LLDP (сосед на порту).
- [x] План п.6: [Incident-Actions-Plan.md](Incident-Actions-Plan.md).

## Расширения (0.5.x)

- [x] Расследователь MAC-аномалий: история `mac_fdb_moves`, `MAC_FLAPPING` / `MAC_MULTI_ACCESS`, отчёт гипотез, опциональный syslog (`docs/MAC-Investigation.md`) (0.5.4)
- [x] Карта перемещений MAC + список flappers на `/investigate/mac` (0.5.5)
- [x] Гипотеза `kvm_dual_uplink` для QEMU/KVM + dual-port flap; RBAC sync-port-roles → operator (0.5.6)

### Эксперт по неисправностям (эволюция investigate)

Развиваем текущий `internal/investigate`, **без** параллельного пакета `expert/`:

- [x] **Фаза 1:** OUI-гипервизоры, `kvm_dual_uplink` (52:54 + два access без LLDP), чек-лист Proxmox/libvirt/STP (0.5.6)
- [x] **Config snapshots + diff** show run: таблица `device_config_snapshots`, UI на карточке узла, scheduler + hook backup/port_sync (0.5.7)
- [x] **STP poller:** BRIDGE-MIB `dot1dStpTopChanges`/root → `STP_TOPOLOGY_CHANGE` / `STP_ROOT_CHANGED` (0.5.8)
- [x] **Фаза 2:** статус расследования MAC (open/resolved/ignored) + UI `/investigate/mac` (0.5.9)
- [x] **Фаза 3:** детектор петель в топологии (DFS по LLDP) — отдельный тип отчёта, не смешивать с MAC flapping (0.5.17)
- [x] Расширяемые правила: rogue MAC, duplicate MAC, port storm — интерфейс `Investigator` поверх `BuildMACReport` (0.5.17)

### Траблшутинг — очередь после 0.5.9 (живой план, сессия 2026-09-01)

Закрыто в **0.5.10:** Eltex `show running-config interfaces` — парсер не «перетекает» в следующий `interface`; модалка порта не показывает shutdown как enable до live-read.

**Порядок реализации (согласовано):**

| # | Задача | Статус | Комментарий |
|---|--------|--------|-------------|
| 1 | **Postmortem** — `GET /api/v1/postmortem`; events + traps + mac_move + config_snapshot; LLDP hops; UI | [x] | 0.5.11; док [Postmortem.md](Postmortem.md). Syslog/метрики uplink в таймлайне — ещё нет |
| 2 | **Traceroute** с сервера NetLynx | [x] | 0.5.12 — POST `/devices/{id}/traceroute`. **MTR нет** |
| 3 | **TCP-probe** | [x] | 0.5.14 |
| 4 | **Broadcast-storm эвристика** | [x] | 0.5.15 |
| 5 | **FDB daily snapshots** | [x] | 0.5.16 |
| 6 | **SNMP Walk UI** | [ ] **отложено** | |
| 7 | **Фаза 3 investigate** — петли DFS по LLDP | [x] | 0.5.17; док [Loop-Investigation.md](Loop-Investigation.md) |
| 8 | **Investigator:** rogue / duplicate / port storm | [x] | 0.5.17 |

**Отложено (по решению):** SNMP Walk UI (п.6) — операторский инструмент, не очередь авто-траблшутинга.

### MAC-tracing — «где физически подключён» (советы 2026-09-01)

Интерпретация `device_fdb_entries`: снимок = **последний** порт на свитче, не «железо воткнуто сюда навечно». Читать InvestigateMAC: роль + `macs_on_port` → timeline → гипотезы → карта.

| # | Задача | Статус | Комментарий |
|---|--------|--------|-------------|
| A | **L2-path** — BFS по LLDP от core/корня до access-порта с MAC | [x] | 0.5.18 — блок «Путь к устройству» |
| B | **Баннер multi-access** — footprint ≥2 access на разных свитчах | [x] | 0.5.18 — красный баннер в UI |
| C | **FDB daily snapshots** | [x] | 0.5.16 |
| D | **Shut порт из InvestigateMAC** | [x] | 0.5.18 + **0.5.19** превью FDB/LLDP и риск uplink |
| — | **Footprint sort + «источник?» + `core_loop_broadcast` + дедуп MULTI_ACCESS** | [x] | 0.5.20 — из внешнего MAC-tracing патча |

**Не в ближайшей очереди:** pcap/NetFlow-коллектор, полный RANCID-замена, cable-test/TDR, L2-path между двумя MAC, ARP-probe (raw socket), SNMP Walk UI.

## Аудит backlog (после 0.17.4 волны 1)

Волна 1 закрыта в 0.17.4: SSE reconnect, tb/radial layout, logout+Basic, NETLYNX_TRUST_PROXY.  
Волна security 0.17.5 + 0.18.0: SSH known_hosts, SSRF+Dial pin, WaitGroup safe, LLDP walks, UISP v3 keep, auth-lost pub/sub.

**Ниже — что из волны 1 ещё в backlog (не «забыли»):**

- [x] Refresh family-detection; JWT random secret in DB (миграция 029)
- [x] Prometheus `/metrics` за auth (0.19.1)
- [x] Backup по расписанию (UI + scheduler, миграция 048)
- [ ] TanStack Query + split giant pages; AbortController everywhere
- [ ] Topology: prev/next match; label collision; group-by-VLAN; edge ARIA
- [ ] Desktop parity / safeStorage / recent_events
- [x] N7 login limiter GC; CookieSecure (`NETLYNX_COOKIE_SECURE`)
- [ ] Last-Event-ID SSE resume; BOM pre-commit в CI; Basic legacy removal
- [x] Settings без обязательного `location.reload` после сохранения (актуально на 0.5.x)

## Аудит 0.17.5 (#6 #7 #10 #15 #16)

- [x] #6 SSH known_hosts (`SSH_POE_KNOWN_HOSTS`, без InsecureIgnoreHostKey)
- [x] #7 SSRF: webhook https-only + block private; UISP LAN private ок, loopback/metadata нет
- [x] #10 WaitGroup: EventHook/Engine + workers до pool.Close
- [x] #15 traprecv → snmp.SanitizeSNMPValue
- [x] #16 LLDP primary BulkWalk error propagates

## Аудит 0.18.0 (остаток PARTIAL + auth-кластер)

- [x] Auth pub/sub: `subscribeAuth` + `auth-lost` / `isLoggingOut` → RequireAuth, SSE stop, api skip refresh (B7+B13+N1.1)
- [x] #7 SSRF end-to-end: `SafeHTTPClient` DialContext pin IP (anti DNS rebinding)
- [x] #16 LLDP: ошибки 2-го/3-го BulkWalk → не писать partial
- [x] N-WG: closed/`tryAdd` перед `Wait` (poller + EventHook)
- [x] #17 UpsertSwitchFromUISP: не даунгрейдить SNMP v3→v2c
- [x] N4.1 Topology auto-refresh: стабильный interval + AbortController + seq
- [x] N-nameMsg: сброс на input change

## Отложено (backlog, см. Roadmap)

Сознательно **не в ближайшем релизе** — продукт уже рабочий; это улучшения и паритет:

- [ ] TanStack Query + split giant pages (DeviceDetail, Settings, …)
- [ ] Desktop parity / safeStorage warning / recent_events
- [ ] Incident actions 2.0 дальше: per-device rules, webhook action, admin_up
- [ ] Last-Event-ID SSE resume; Basic legacy removal
- [ ] Topology multi-neighbor bundling (остаток)
- [ ] BOM check в CI (локально: `scripts/check-bom.sh`, `.githooks`)

### Роль порта / VLAN (после 0.37.33)

В **0.37.33** — роль access/trunk из `show run` по SSH, `cli_port_mode` в БД, poller не затирает CLI-роль; access VLAN из конфига; scroll к порту в таблице. Ниже — **сознательно не в том релизе**:

- [ ] **SNMP PVID** (`dot1qPvid` / Q-BRIDGE): читать native/access VLAN по SNMP как дополнение или fallback без SSH (не замена show run «из коробки» — на EdgeSwitch надёжнее CLI).
- [ ] **Ручной override роли** в UI (access/trunk без правки running-config на свитче): для узлов без SSH или когда конфиг не совпадает с реальностью; нужна политика приоритета UI vs CLI vs poller.
- [ ] **Trunk allowed VLANs**: парсинг `switchport trunk allowed vlan …` (и аналоги) + отдельная колонка или подсказка в таблице портов (не один «домinant VLAN» из FDB).

## Аудит 0.19.1 (топ HIGH из независимого аудита)

- [x] F2 DeviceDetail form-sync только при смене id
- [x] H6/H7 секреты не в JSON (`has_*`, community redact)
- [x] H3/H4 RealIP только TrustProxy; Basic ConstantTimeCompare + loginLimiter
- [x] F6 retry-401 → notifyAuthLost
- [x] H8 family-revoke error не swallow
- [x] H1 `/metrics` за auth (viewer+)
- [x] F1 ErrorBoundary; H5 audit на 6 mutation handlers

## Релиз 0.37.x (документация, notify, security)

- [x] LICENSE **freeware** + указание авторства; README/ТЗ (0.37.0→0.37.29)
- [x] Runbook: шаг 5 (:8080 vs nginx); Ctrl-C/V чеклист → `deploy.sh` (0.37.4–0.37.5)
- [x] README: SNMP RW для управления; контакт jdoe@gmail.com (0.37.3, 0.37.7–0.37.8)
- [x] Журнал версий только example/home; GitHub без changelog в README (0.37.1)
- [x] DEVICE_OFFLINE/ONLINE: recovery после рестарта/выходных; без ложного ONLINE (0.37.9–0.37.14)
- [x] Telegram HTTP 429 Retry-After; email batch при остановке службы (0.37.13)
- [x] Security: `psql` restore без `\!`; Postgres 127.0.0.1:5433; лимит body; incident cooldown; без пароля в sessionStorage (0.37.12)
- [x] Audit: смена роли/пароля в журнале; MikroTik `$` в comment; SSRF preview/email-test (0.37.15)
- [x] Roadmap/To-Do/TZ §12: сверка с 0.37.15; git workflow example → GitHub
- [x] Иконки типов устройств в UI и email (0.37.16)
- [x] Роль порта из show run (SSH), scroll к порту в карточке узла (0.37.33)
- [x] **0.5.48** — сверка `/docs` с кодом: Roadmap/ТЗ/API/Roles; доки Петли и Postmortem; правки Autodiscover/Incident/PoE/Ignore/MAC/…
- [x] **0.5.49** — карточка коммутатора: закладки info/snmp/state/vlan-stub/ports (`?tab=`); W3 без срока; VLAN database CRUD — next
- [x] **0.5.50** — снимки конфига: игнор uptime / SNTP / `ntp clock-period` / шапка RouterOS (без ложных бэкапов)
- [x] **0.5.51** — VLAN tab: чтение vlan database + порты/FDB; VLAN на порту (cisco `switchport` → 802.1Q)
- [x] **0.5.52** — VLAN tab: не затирать полный список локальным infer после sync портов
- [x] **0.5.53** — имя и удаление VLAN в vlan database свитча (cisco `vlan N` → Fastpath `vlan database`); VLAN 1 не удаляется
- [x] **0.5.54** — EdgeSwitch: имя VLAN из privileged `vlan database` + кавычки для дефисов
- [x] **0.5.55** — VLAN database: порядок CLI по вендору (ELTEX/Huawei/Cisco/Fastpath); парсинг `vlan N name`
- [x] **0.5.56** — VLAN tab: перерисовка таблицы после delete/name (vlans в ответе API + retry show run)
- [x] **0.5.57** — VLAN tab: статус «Идёт обновление конфига…» на время SSH/записи
- [x] **0.5.58** — создание VLAN в vlan database; trunk Allow vlan (add/remove/all/except) в UI
- [x] **0.5.59** — массовое удаление VLAN (чекбоксы → `no vlan A,B,C`)
- [x] **0.5.60** — VLAN: · FDB vs · с портов; блок удаления пока VLAN на портах
- [x] **0.5.61** — предупреждение: trunk Allow vlan без VLAN управления (риск потери SSH/Web)
- [x] **0.5.62** — не показывать VLAN-призраков только из FDB после удаления из vlan database
- [x] **0.6.0** — релиз линии VLAN (database CRUD + trunk Allow + UX/guardrails 0.5.49–0.5.62)
- [ ] **MikroTik CRS/RouterOS VLAN** — чтение/запись `/interface bridge vlan` (+ comment как «имя»); VLAN на порту (tagged/untagged); не IOS vlan database
- [ ] **Trunk allowed VLANs** — колонка/подсказка в таблице портов (парсинг show run уже частично есть; запись — 0.5.58)

