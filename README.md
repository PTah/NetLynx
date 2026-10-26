# NetLynx

SNMP-мониторинг коммутаторов, веб-UI.

Программа **следит за коммутаторами по SNMP**: опрашивает их, показывает состояние и события (линк up/down, загрузка порта и т.д.) в **веб-интерфейсе** в браузере.

Приложение разрабатывалось для личных нужд в офисе — в основном для поиска различных устройств в сети и для построения топологии. Затем разрослось до того, что получилось :)

Техническое задание: [docs/TZ-snmp-switch-monitor.md](docs/TZ-snmp-switch-monitor.md). HTTP API: [docs/API.md](docs/API.md).

**English:** [README.en.md](README.en.md)

**Скриншоты:** [SCREENSHOTS.md](SCREENSHOTS.md)

**Разделы:** [Установка](#установка-с-нуля) · [Первый запуск](#первый-запуск-после-установки) · [Что умеет](#что-умеет-netlynx) · [Настройки окружения](#настройки-окружения-env) · [Обновление и эксплуатация](#обновление-и-эксплуатация) · [Последние версии](#последние-версии-кратко) · [Журнал версий](docs/Changelog.md) · [Состав](#из-чего-состоит-netlynx)

**Документация:** индекс всех документов — [docs/README.md](docs/README.md); английские версии — [docs/en/](docs/en/) (перевод добавляется партиями, статус в таблице индекса).

## Что умеет NetLynx

| Возможность | Коротко | Документ |
|---|---|---|
| Опрос по SNMP | порты, скорость, утилизация, CPU, события Link Up/Down, flap, broadcast storm; лёгкий опрос каждые 15–60 с, тяжёлый (LLDP/CDP/ARP/PoE/STP/FDB) — раз в 5 мин | [Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md) |
| Топология | LLDP/CDP + FDB, карта, ручные линки, поиск пути, blast-radius (VLAN / shutdown порта) | [Manual-Topology-Links.md](docs/Manual-Topology-Links.md) |
| Обнаружение и инвентарь | подсеть по SNMP/ICMP, «Обнаружено» → «Узлы», типы устройств, UISP | [Autodiscover.md](docs/Autodiscover.md) |
| Порты и VLAN | access/trunk/native/allowed, VLAN database, live-настройки порта по SSH, PoE (детект, ватты, reset / расписание), mass-edit; FDB «кто за портом» (в т.ч. SNR ifIndex 5001+) | [VLAN.md](docs/VLAN.md), [PoE-detection.md](docs/PoE-detection.md), [Vendors.md](docs/Vendors.md) |
| Расследования | где MAC, почему петля, кто за портом | [MAC-Investigation.md](docs/MAC-Investigation.md), [Loop-Investigation.md](docs/Loop-Investigation.md) |
| МФУ / принтеры | счётчик страниц, остаток тонера, графики | [Printer-MIB.md](docs/Printer-MIB.md) |
| ИБП | UPS-MIB: заряд, автономия, вход/выход, нагрузка, температура, аварии; события и история | [UPS.md](docs/UPS.md) |
| События и уведомления | журнал событий, Telegram / email / webhook, фильтры по типам и важности, статистика offline и питания ИБП | [Notifications.md](docs/Notifications.md) |
| Роли и доступ | viewer / operator / admin, JWT + сессии, Basic как аварийный вход | [Roles.md](docs/Roles.md) |
| Секреты и бэкапы | AES-GCM для community/SSH/SMTP/токенов; ZIP-бэкап БД + env; снимки `show run` и MikroTik `/export` | [Secrets.md](docs/Secrets.md), [Config-History.md](docs/Config-History.md) |

## Настройки окружения (env)

Файл: `/etc/netlynx/netlynx.env` на Linux (шаблон — [.env.example](.env.example)). После правки: `sudo systemctl restart NetLynx`. Секреты (community, SMTP, токены) хранятся в БД зашифрованными — см. [Secrets.md](docs/Secrets.md).

| Переменная | По умолчанию | Зачем |
|---|---|---|
| `DATABASE_URL` | `postgres://…@127.0.0.1:5433/invetor` | PostgreSQL (БД называется `invetor` — исторически) |
| `HTTP_ADDR` | `:8080` | адрес API + веб-UI (nginx опционально) |
| `NETLYNX_ADMIN_USER` / `NETLYNX_ADMIN_PASSWORD` | `admin` / пароль из файла | первый вход; далее — пользователи в UI |
| `NETLYNX_SECRETS_KEY` | — | ключ AES-GCM для секретов в БД (`secrets-genkey`) |
| `POLL_SCHEDULER_SECONDS` | `10` | тик планировщика опроса |
| `POLL_CRITICAL_INTERVAL_SECONDS` | `15` | верхняя граница лёгкого опроса для роутеров/коммутаторов/серверов |
| `POLL_HEAVY_SECONDS` | `300` | тяжёлый опрос (LLDP/CDP/ARP/PoE/STP/FDB); можно переопределить в карточке узла |
| `FDB_POLL_INTERVAL_SECONDS` | `900` | опрос FDB (внутри тяжёлого) |
| `METRICS_ENABLED` / `METRICS_RETENTION_DAYS` | `true` / `7` | история метрик: вкл/выкл и срок хранения |
| `METRICS_UTIL_INTERVAL_SECONDS` | `60` | как часто писать util/rate в историю |
| `METRICS_PRINTER_RETENTION_DAYS` | `365` | хранение тонера/страниц/ИБП (`page_count`, `toner_*`, `ups_*`) |
| `WEB_DIST` | `web/dist` | каталог собранного веба |
| `NETLYNX_COOKIE_SECURE` | `false` | `true` при работе только по HTTPS |

Полный список с комментариями — в [.env.example](.env.example).

## Обновление и эксплуатация

- **Обновить версию:** [Runbook-Linux.md → «Обновление версии»](docs/Runbook-Linux.md#обновление-версии) (git pull + `bash docs/deploy.sh`; миграции БД применяются автоматически при старте).
- **Откатиться:** [Приложение C](docs/Runbook-Linux.md#приложение-c-откат-на-старую-версию).
- **Проверить, что живо:** `curl -s http://127.0.0.1:8080/health` → `{"status":"ok","version":"…"}`.
- **Логи службы:** `journalctl -u NetLynx -f` или **Настройки → Журнал** в UI.
- **Резервные копии:** **Настройки → Резервные копии** + [Config-History.md](docs/Config-History.md).
- **Если не работает:** [Runbook → «Если что-то пошло не так»](docs/Runbook-Linux.md#если-что-то-пошло-не-так) (служба, Postgres, порт, сборка web).

## Первый запуск после установки

Пятнадцать минут, чтобы система стала полезной (всё — в UI, роль admin):

1. **Смените пароль администратора** — «Пользователи» → `admin` → пароль (или сразу создайте личные учётки: [Roles.md](docs/Roles.md)).
2. **Проверьте секреты.** Community/SSH-пароли/SMTP/токены шифруются ключом `NETLYNX_SECRETS_KEY` — он уже создан установщиком; не теряйте его, без него секреты в БД не расшифруются ([Secrets.md](docs/Secrets.md)).
3. **Настройте SNMP на коммутаторах**: RO-community для чтения (RW — только если нужны VLAN/PoE-изменения), `snmp-server host <IP NetLynx>` для трапов ([SNMP-Community.md](docs/SNMP-Community.md)).
4. **Включите LLDP/CDP на свитчах** — без этого не будет топологии и автообнаружения ([Autodiscover.md](docs/Autodiscover.md)).
5. **Добавьте узлы**: «Обнаружено» → отметить → «В узлы», либо вручную (имя, IP, тип, community). Тип узла влияет на приоритет опроса: роутеры/коммутаторы/серверы опрашиваются каждые 15 с.
6. **Проверьте карточку одного коммутатора**: порты, VLAN, PoE, топология — если пусто, сверьте vendor/SSH и community ([Vendors.md](docs/Vendors.md)).
7. **Настройте уведомления**: Telegram/email/webhook, отметьте нужные типы событий ([Notifications.md](docs/Notifications.md)).
8. **Настройте резервные копии** (Настройки → Резервные копии) и проверьте, что файл создаётся.

## Установка с нуля

| Платформа | Документ |
|-----------|----------|
| **Linux** (рекомендуется) | [docs/Runbook-Linux.md](docs/Runbook-Linux.md) — сервер с нуля; вход в UI: [шаг 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) (`:8080` или nginx) |
| **Linux, уровень Ctrl-C/Ctrl-V** *(админ с Habr)* | [Чеклист](docs/Runbook-Linux.md#краткий-чеклист-копируйте-блоками) — копируйте **весь блок** (пакеты → docker → clone → `bash docs/deploy.sh`). [deploy.sh](docs/deploy.sh) **ставит всё сам** (сборка, Postgres, systemd, health). [Шаг 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) — войти в UI. Runbook — если пропустили перелогин после `usermod docker` *(спойлер: `permission denied` — это не баг NetLynx)*. |
| **Windows Server** | [docs/Windows-Server-Setup.md](docs/Windows-Server-Setup.md) |

После установки: SNMP community — [docs/SNMP-Community.md](docs/SNMP-Community.md); автообнаружение (нужны **SNMP + LLDP** на свитчах) — [docs/Autodiscover.md](docs/Autodiscover.md); опросы и кэш топологии (поллер, «ночной» blast-rebuild) — [docs/Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md); вендоры — [docs/Vendors.md](docs/Vendors.md); МФУ/принтеры (страницы, тонер) — [docs/Printer-MIB.md](docs/Printer-MIB.md); ИБП по SNMP — [docs/UPS.md](docs/UPS.md); уведомления (Telegram/email/webhook) — [docs/Notifications.md](docs/Notifications.md).

Эталонный репозиторий git: [https://github.com/PTah/NetLynx](https://github.com/PTah/NetLynx) (ветка `main`).

Эксплуатационные документы:
- Роли доступа: [docs/Roles.md](docs/Roles.md)
- PoE и SNMP (диагностика): [docs/PoE-detection.md](docs/PoE-detection.md)
- **SNMP RO/RW** (управление портами): [docs/SNMP-Community.md](docs/SNMP-Community.md) — мониторинг работает с **RO**; **управлять** коммутатором (shutdown, alias, incident actions) можно только с **RW** community на свитче — иначе NetLynx лишь смотрит, и в этом страшном мире крутым не быть.
- **Автообнаружение / топология LLDP:** [docs/Autodiscover.md](docs/Autodiscover.md) — на свитчах должны быть включены **SNMP** и **LLDP** (NetLynx их не включает).
- **Опросы и кэш топологии:** [docs/Polling-and-Topology-Cache.md](docs/Polling-and-Topology-Cache.md) — непрерывный SNMP-poll, ночной/часовой blast-rebuild, FDB/config snapshots; ручной скан подсети — из Настроек (не ночной sweep).
- **Секреты at-rest:** [docs/Secrets.md](docs/Secrets.md) — AES-GCM для community/SSH/SMTP/токенов (`NETLYNX_SECRETS_KEY`, `secrets-rewrap`).
- **История конфигов и бэкапы:** [docs/Config-History.md](docs/Config-History.md) — снимки `show run`, бэкап `show run` / MikroTik `/export` в ZIP (`backup_router_cfg`).
- Производители коммутаторов: [docs/Vendors.md](docs/Vendors.md)
- Расследование MAC / flapping: [docs/MAC-Investigation.md](docs/MAC-Investigation.md)
- Уведомления (Telegram / email / webhook), фильтры, типы событий: [docs/Notifications.md](docs/Notifications.md)
- Петли LLDP: [docs/Loop-Investigation.md](docs/Loop-Investigation.md)
- Postmortem: [docs/Postmortem.md](docs/Postmortem.md)
- HTTPS через nginx (опционально): [docs/install-nginx.sh](docs/install-nginx.sh)
- Desktop Windows: [desktop/README.md](desktop/README.md) — **не готов**, только черновик под ТЗ
- План / backlog: [docs/Roadmap.md](docs/Roadmap.md), [docs/To-Do.md](docs/To-Do.md)

**Журнал версий:** при **заметных** изменениях (новая возможность UI, новый SNMP/MIB, миграция БД, смена поведения API) в том же изменении, где поднимается **`VERSION`**, добавьте запись в [docs/Changelog.md](docs/Changelog.md) и **продублируйте её** в блок «Последние версии (кратко)» ниже (1–5 пунктов для пользователя). Мелкие правки и рефакторинг в журнал не обязательны — их смотрят по `git log`.

### Нумерация версий (с 0.2.0)

Формат: **`0.<minor>.<patch>`** (файл `VERSION` в корне).

| Правило | Как |
|--------|-----|
| Обычная правка | **patch** +1: `0.2.0` → `0.2.1` → … → `0.2.9` |
| После `.9` | следующий **minor**, patch с нуля: `0.2.9` → **`0.3.0`**, затем `0.3.1`…`0.3.9` → **`0.4.0`** и т.д. |
| Крупный сдвиг | по смыслу можно поднять **major** (`1.0.0`), отдельно от этой лестницы |

История до сброса (линия `0.13.x` и раньше) остаётся в журнале — [docs/Changelog.md](docs/Changelog.md); текущая линия начинается с **0.2.0**.

### Последние версии (кратко)

Полный журнал всех версий — [docs/Changelog.md](docs/Changelog.md). Ниже — последние 8 релизов.

**0.6.4** — вкл/выкл порта и description: сначала SSH + `write` в startup (не только SNMP runtime).

**0.6.3** — События: поиск узла по имени (не только select), фильтр периода «С»/«По».

**0.6.2** — remote syslog-архив (`/var/lib/remotelogs`), fan-out в `:9514`; см. [docs/Remote-Syslog.md](docs/Remote-Syslog.md).

**0.6.1** — события SSH-конфига без мусорного баннера в тексте; диагностика Linux MOTD / не-CLI.

**0.6.0** — топология: заливка карточки по цвету типа (NVR и др.); last-known связи offline 30 дней (UISP-style).

**0.5.9** — docs: VLAN/API/Vendors/Polling приведены к поведению 0.5.6–0.5.8 (ответ «Готово», live settings, FDB SNR).

**0.5.8** — FDB на SNR: SNMP-маппинг ifIndex 5001–5009 (ge1–ge9), иначе таблица MAC была пустой при живом CLI.

**0.5.7** — настройки порта SNR открываются чтением одного интерфейса, без зависания на `--More--`.

**0.5.6** — вкладка VLAN: зелёное «Готово» сразу после записи на коммутатор, не после долгого show run. Если ответ затянулся, под кнопкой прямо сказано, что повторно нажимать нельзя.

**0.5.5** — порт: **Сброс PoE по расписанию** (раз в сутки или N раз начиная с времени; 3× с 03:00 → 03:00/11:00/19:00).

**0.5.4** — SNMP-backoff не глушит коммутаторы: «Проверить SNMP» обновляет статус и запускает опрос (раньше UI залипал на timeout при живом SNMP).

**0.5.3** — приоритеты мониторинга по типу (критичные → NAS → ИБП/ПК → AP/телефоны → камеры/МФУ); антидребезг offline для «мягких» узлов; в карточке — **«Подключен к…»** (авто LLDP/CDP/FDB) перед «Ручные связи».

**0.5.2** — документация, партия 4 (весь эксплуатационный блок переведён): [Autodiscover](docs/en/Autodiscover.md), [Vendors](docs/en/Vendors.md), [PoE-detection](docs/en/PoE-detection.md) — **19 из 30** документов на английском, включая все руководства для админа и инженера.

**0.5.1** — документация, партия 3 перевода: [Windows-Server-Setup](docs/en/Windows-Server-Setup.md), [Config-History](docs/en/Config-History.md), [Polling-and-Topology-Cache](docs/en/Polling-and-Topology-Cache.md), [Ignore-List](docs/en/Ignore-List.md), [MAC-Investigation](docs/en/MAC-Investigation.md) — всего **16 из 30** документов на английском. Заодно исправлена склеенная строка таблицы «лёгкий/тяжёлый опрос» в русском `Polling-and-Topology-Cache.md`.

**0.5.0** — документация, партия 2 перевода: [VLAN](docs/en/VLAN.md), [Notifications](docs/en/Notifications.md), [Secrets](docs/en/Secrets.md), [Printer-MIB](docs/en/Printer-MIB.md), [Loop-Investigation](docs/en/Loop-Investigation.md), [Manual-Topology-Links](docs/en/Manual-Topology-Links.md) — всего 11 документов на английском.

**0.4.9** — документация: английские версии пяти ключевых документов — [Runbook-Linux](docs/en/Runbook-Linux.md), [API](docs/en/API.md), [Roles](docs/en/Roles.md), [SNMP-Community](docs/en/SNMP-Community.md), [UPS](docs/en/UPS.md); двуязычный индекс со статусом перевода — [docs/README.md](docs/README.md); в русских документах появилась ссылка на английскую версию.

**0.4.8** — документация: журнал версий вынесен в [docs/Changelog.md](docs/Changelog.md) (в README — последние релизы), добавлен чек-лист **«Первый запуск после установки»**, новый [docs/VLAN.md](docs/VLAN.md) (база VLAN, access/trunk/native/allowed, проверка последствий удаления, ограничения) и полный английский [README.en.md](README.en.md).

**0.4.7** — документация для админов: в README добавлены разделы **«Что умеет»**, **«Настройки окружения (env)»**, **«Обновление и эксплуатация»** и оглавление; новый [docs/Notifications.md](docs/Notifications.md) — Telegram/email/webhook, фильтры, полный список типов событий, статистика и чеклист «почему не приходит»; исправлены битые ссылки в `desktop/README.md`.

**0.4.6** — Статистика уведомлений: периоды от **1 часа** до **365 дней** (1–12/24/48/72 ч, 7…365 дн.); API принимает `hours` (до 8760).

**0.4.5** — Настройки → Уведомления → **Статистика**: таблица по устройствам за период — сколько раз уходили в оффлайн (`DEVICE_OFFLINE`/`DEVICE_ONLINE`) или сколько раз у ИБП пропадало питание (`UPS_ON_BATTERY`/`UPS_MAINS_OK`); API `GET /api/v1/settings/notifications/stats`.

**0.4.4** — журнал: повторные SNMP-сбои одного эпизода (узлы без агента) пишутся в debug, warn остаётся только на первом сбое; FDB-попытка отмечается и при ошибке, поэтому «bridge port map empty» теперь не чаще раза в `FDB_POLL_INTERVAL_SECONDS` на узел (было каждые 5 минут).

**0.4.3** — интервал тяжёлого опроса можно задать в карточке узла («Тяжёлый опрос (с)», `PATCH /devices/{id}/monitoring`, в БД `NULL` = `POLL_HEAVY_SECONDS`). Неудачный опрос FDB теперь тоже отмечается (`last_fdb_poll_at`), поэтому узлы без bridge-MIB не пробуются каждые 5 минут и не засоряют журнал `bridge port map empty`.

**0.4.2** — util/rate пишутся в историю не чаще `METRICS_UTIL_INTERVAL_SECONDS` (по умолчанию 60 с): после перехода на 15-секундный лёгкий опрос в `metric_samples` уходило ~19 млн строк/сутки, с троттлингом — ~10 млн (уровень до перехода, ~1 семпл на порт в минуту). Статусы портов, LINK_UP/DOWN и flap-детект по-прежнему работают каждые 15–20 с.

**0.4.1** — SNMP-backoff для узлов без SNMP-агента (камеры, ПК, ТВ): после 3 сбоев подряд SNMP не опрашивается 15 минут, ICMP продолжает работать — меньше таймаутов в журнале, слоты опроса не простаивают (было ~70 неудачных SNMP-опросов в минуту).

**0.4.0** — лёгкий и тяжёлый опрос разделены: критичные типы (роутер, коммутатор, сервер) опрашиваются не реже **15 с** (`POLL_CRITICAL_INTERVAL_SECONDS`), остальные — по своему интервалу; тяжёлые таблицы (LLDP/CDP/ARP/PoE/STP/chassis/FDB, роли портов) — раз в `POLL_HEAVY_SECONDS` (300 с по умолчанию) вместо каждого опроса. Очередь: сначала критичные типы, внутри — самые просроченные. PoE-флаги и роли портов на лёгком опросе не перезаписываются. Ожидаемый эффект: падение коммутатора видно в пределах ~15–30 с, нагрузка на сеть и коммутаторы ниже.

## Из чего состоит NetLynx

1. **Сервер (Go)** — опрос SNMP, API, PostgreSQL.
2. **Веб (React)** — дашборд, узлы, события, настройки; отдаётся с того же порта **8080**, что и API.

**Лицензия:** freeware — бесплатно для личного и некоммерческого использования на условиях [LICENSE](LICENSE). Указание авторства обязательно.
