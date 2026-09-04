# NetLynx

SNMP-мониторинг коммутаторов, веб-UI.

Программа **следит за коммутаторами по SNMP**: опрашивает их, показывает состояние и события (линк up/down, загрузка порта и т.д.) в **веб-интерфейсе** в браузере.

Приложение разрабатывалось для личных нужд в офисе — в основном для поиска различных устройств в сети и для построения топологии. Затем разрослось до того, что получилось :)

Техническое задание: [docs/TZ-snmp-switch-monitor.md](docs/TZ-snmp-switch-monitor.md). HTTP API: [docs/API.md](docs/API.md).

**English:** [README.en.md](README.en.md)

**Скриншоты:** [SCREENSHOTS.md](SCREENSHOTS.md)

## Установка с нуля

| Платформа | Документ |
|-----------|----------|
| **Linux** (рекомендуется) | [docs/Runbook-Linux.md](docs/Runbook-Linux.md) — сервер с нуля; вход в UI: [шаг 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) (`:8080` или nginx) |
| **Linux, уровень Ctrl-C/Ctrl-V** *(админ с Habr)* | [Чеклист](docs/Runbook-Linux.md#краткий-чеклист-копируйте-блоками) — копируйте **весь блок** (пакеты → docker → clone → `bash docs/deploy.sh`). [deploy.sh](docs/deploy.sh) **ставит всё сам** (сборка, Postgres, systemd, health). [Шаг 5](docs/Runbook-Linux.md#шаг-5-первый-вход-в-браузере) — войти в UI. Runbook — если пропустили перelogin после `usermod docker` *(спойлер: `permission denied` — это не баг NetLynx)*. |
| **Windows Server** | [docs/Windows-Server-Setup.md](docs/Windows-Server-Setup.md) |

После установки: SNMP community — [docs/SNMP-Community.md](docs/SNMP-Community.md); автообнаружение (нужны **SNMP + LLDP** на свитчах) — [docs/Autodiscover.md](docs/Autodiscover.md); вендоры — [docs/Vendors.md](docs/Vendors.md).

Эталонный репозиторий git: [https://github.com/PTah/NetLynx](https://github.com/PTah/NetLynx) (ветка `main`).

Эксплуатационные документы:
- Роли доступа: [docs/Roles.md](docs/Roles.md)
- PoE и SNMP (диагностика): [docs/PoE-detection.md](docs/PoE-detection.md)
- **SNMP RO/RW** (управление портами): [docs/SNMP-Community.md](docs/SNMP-Community.md) — мониторинг работает с **RO**; **управлять** коммутатором (shutdown, alias, incident actions) можно только с **RW** community на свитче — иначе NetLynx лишь смотрит, и в этом страшном мире крутым не быть.
- **Автообнаружение / топология LLDP:** [docs/Autodiscover.md](docs/Autodiscover.md) — на свитчах должны быть включены **SNMP** и **LLDP** (NetLynx их не включает).
- Производители коммутаторов: [docs/Vendors.md](docs/Vendors.md)
- Расследование MAC / flapping: [docs/MAC-Investigation.md](docs/MAC-Investigation.md)
- Петли LLDP: [docs/Loop-Investigation.md](docs/Loop-Investigation.md)
- Postmortem: [docs/Postmortem.md](docs/Postmortem.md)
- HTTPS через nginx (опционально): [docs/install-nginx.sh](docs/install-nginx.sh)
- Desktop Windows: [desktop/README.md](desktop/README.md) — **не готов**, только черновик под ТЗ
- План / backlog: [docs/Roadmap.md](docs/Roadmap.md), [docs/To-Do.md](docs/To-Do.md)

## Из чего состоит NetLynx

1. **Сервер (Go)** — опрос SNMP, API, PostgreSQL.
2. **Веб (React)** — дашборд, узлы, события, настройки; отдаётся с того же порта **8080**, что и API.

**Лицензия:** freeware — бесплатно для личного и некоммерческого использования на условиях [LICENSE](LICENSE). Указание авторства обязательно.
