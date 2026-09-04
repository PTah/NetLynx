# SNMP Community в NetLynx

Как настроить опрос и управление коммутаторами по SNMPv1/v2c (и кратко — v3).

## Зачем

NetLynx опрашивает узлы по SNMP (интерфейсы, LLDP/CDP, FDB, PoE, CPU и т.д.).  
Часть операций **пишет** на устройство через SNMP SET:

- вкл/выкл порта (admin up/down);
- описание порта (`ifAlias`);
- авто-блокировка порта при инцидентах (если включено в Настройках).

Для **чтения** достаточно community с правами **read-only (RO)**.  
Для **записи** на коммутаторе нужна community с правами **read-write (RW)** — и то же значение должно быть указано в карточке узла NetLynx.

> В UI узла одно поле **Community** — оно используется и для GET, и для SET. Отдельного поля «RW community» нет: если нужны SET, в это поле указывают **RW** community (или SNMP v3 с правами записи).

Подробнее про авто-действия: [Incident-Actions-Plan.md](Incident-Actions-Plan.md).  
Для части вендоров (Ubiquiti / ELTEX / SNR) управление портом может идти по **SSH** — тогда RW community не обязателен для этих операций, но для опроса SNMP community (хотя бы RO) всё равно нужна.

## На коммутаторе

1. Включите SNMP (агент / SNMP server).
2. Создайте community:
   - **RO** — только мониторинг (безопаснее для «только смотреть»).
   - **RW** — если планируете shutdown / описание порта / incident admin down из NetLynx.
3. Ограничьте доступ по ACL/source IP до сервера NetLynx (рекомендуется).
4. Не используйте `public`/`private` на production без ACL.
5. Для **автообнаружения** и топологии по соседям на тех же свитчах включите **LLDP** (на портах; у Cisco — при необходимости ещё CDP). NetLynx сам SNMP/LLDP на железе не включает — см. [Autodiscover.md](Autodiscover.md).

Примеры смысла (синтаксис зависит от вендора):

| Цель | На свитче | В NetLynx |
|------|-----------|-----------|
| Только мониторинг | community `netlynx-ro` RO | Community = `netlynx-ro` |
| Мониторинг + управление портом по SNMP | community `netlynx-rw` RW | Community = `netlynx-rw` |
| SNMPv3 | user с auth/priv; для SET — write views | Версия v3 + user/auth/priv в карточке |

## В NetLynx

1. **Узлы** → карточка устройства (или форма добавления / «Обнаружено» → Добавить).
2. SNMP: **v1** / **v2c** / **v3**.
3. Для v1/v2c: поле **Community** = то же имя, что на свитче (RO или RW — см. таблицу выше).
4. Сохраните; дождитесь опроса (или «Проверить SNMP»).
5. Если SET не проходит, в логах/ошибках часто: «community без write?» / «нужен write community или SSH» — проверьте RW на свитче или настройте SSH в карточке узла (где поддерживается).

Trap community (приём SNMP trap на сервере) — это **другое** значение, в env (`SNMP_TRAP_COMMUNITY`), не community узла.
Включение приёма и UDP-порт: **Настройки → Уведомления → Принимать traps** (по умолчанию порт **9162**; БД + hot-reload). Env `SNMP_TRAP_LISTEN_ADDR` listener’ом не управляет. Если порт &gt; 1000 — DNAT **162→этот порт** при необходимости.

**Trap logs (test):** вкладка активна только при включённом приёме. Пока «Вести trap логи» включено, **не-link** traps **не** дублируются в общий журнал как `SNMP_TRAP`. Для **LINK**: режим `link_trap_events_mode` (`off` / `per_device` / `all`) — можно сразу писать `LINK_UP`/`LINK_DOWN` из trap; poller потом может выставить `trap_confirmed`. Режим `off` — только журнал traps, LINK в событиях после опроса.

## Минимальные права (рекомендация)

- Мониторинг парка: RO community + ACL.
- Управление отдельными узлами: RW только на нужных свитчах, или SSH-учётка с ограниченными командами.
- Incident auto admin-down: только при явном включении в Настройках; сначала dry-run; RW обязателен для SNMP SET.

## Связанные документы

- [Runbook-Linux.md](Runbook-Linux.md) — развёртывание сервера
- [Autodiscover.md](Autodiscover.md) — LLDP/CDP, «Обнаружено», требования на свитче
- [Vendors.md](Vendors.md) — поддержка производителей
- [Roles.md](Roles.md) — кто может менять узлы и включать действия
- [PoE-detection.md](PoE-detection.md) — опрос PoE
