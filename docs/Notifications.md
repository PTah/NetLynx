# Уведомления: Telegram, email, webhook

**Перевод:** [English version](en/Notifications.md)

NetLynx пишет **все** события в журнал (UI: «Журнал»). Дополнительно их можно отправлять в каналы: **Telegram**, **email**, **webhook**. Настройка: **Настройки → Уведомления** (роль operator и выше).

Коротко: событие → фильтр по типу и важности → канал → при ошибке повтор (retry).

## Каналы

| Канал | Что задать | Куда смотреть при ошибке |
|---|---|---|
| **Telegram** | включить канал, `bot_token` (создать бота у @BotFather), `chat_id` (ID чата/группы; бот должен быть участником) | журнал службы: `journalctl -u NetLynx \| grep telegram` |
| **Email (SMTP)** | включить, `smtp_host`/`smtp_port` (587 по умолчанию), логин/пароль, `from`, `to` (несколько — через запятую), при самоподписанном сертификате — «не проверять TLS» | `journalctl -u NetLynx \| grep -i smtp`; кнопка **«Проверить email»** (`POST /api/v1/settings/notifications/email-test`) |
| **Webhook** | включить, URL (проверяется политика исходящих — SSRF-защита), фильтры | `journalctl -u NetLynx \| grep webhook`; ответ получателя должен быть 2xx |

Письма о **DEVICE_OFFLINE / DEVICE_ONLINE** группируются в одно письмо (батч), остальные события уходят по одному.

Секреты (токен бота, SMTP-пароль) хранятся в БД зашифрованными — см. [Secrets.md](Secrets.md).

## Фильтры

У каждого канала свои:

- **Типы событий** (`event_types`, список через запятую) — **пустое значение = все типы**.
- **Важность** (`severities`: `info`, `warning`, `error`) — пусто = все.

Повторы: `notify_max_retries` (по умолчанию 2) и `notify_retry_backoff_ms` (500 мс). После исчерпания попыток событие остаётся в журнале, а ошибка — в журнале службы (Warn).

## Типы событий

| Группа | Типы |
|---|---|
| Порты и связь | `LINK_UP`, `LINK_DOWN`, `PORT_FLAP`, `PORT_SPEED_DOWN`, `PORT_SPEED_OK`, `PORT_UTILIZATION_HIGH`, `PORT_UTILIZATION_OK` |
| Узел | `DEVICE_OFFLINE`, `DEVICE_ONLINE`, `SERVICE_STARTED`, `CONFIG_EDIT`, `SNMP_TRAP` |
| MAC / L2 | `UNKNOWN_MAC_ON_ACCESS_PORT`, `MAC_MOVED`, `MAC_FLAPPING`, `MAC_MULTI_ACCESS`, `MAC_REMOVED`, `ACCESS_PORT_MAC_SUBSTITUTED`, `ACCESS_PORT_LONG_IDLE_DEVICE` |
| Петли и STP | `L2_LOOP_APPEARED`, `STP_TOPOLOGY_CHANGE`, `STP_ROOT_CHANGED`, `BROADCAST_STORM_SUSPECTED` |
| Топология | `MANUAL_LINK_SUPERSEDED` |
| МФУ | `TONER_LOW`, `TONER_CRITICAL` |
| ИБП | `UPS_ON_BATTERY`, `UPS_MAINS_OK`, `UPS_ON_BATTERY_LONG`, `UPS_BATTERY_LOW`, `UPS_BATTERY_DEPLETED`, `UPS_BATTERY_OK`, `UPS_CHARGE_LOW`, `UPS_CHARGE_CRITICAL`, `UPS_CHARGE_OK`, `UPS_CHARGE_DROP`, `UPS_DISCHARGE_FAST`, `UPS_RUNTIME_LOW`, `UPS_TEMP_HIGH`, `UPS_TEMP_CRITICAL`, `UPS_ALARM`, `UPS_ALARM_CLEAR`, `UPS_LOAD_HIGH`, `UPS_LOAD_OK` |

Смысл событий ИБП и пороги — [UPS.md](UPS.md). События портов и MAC — [Polling-and-Topology-Cache.md](Polling-and-Topology-Cache.md), [MAC-Investigation.md](MAC-Investigation.md).

## Статистика (с 0.4.5)

**Настройки → Уведомления → Статистика** — таблица по устройствам за период: сколько раз узел уходил в оффлайн или сколько раз у ИБП пропадало питание.

- Периоды: от **1 часа** до **365 дней** (1–12/24/48/72 ч, 7…365 дн.).
- API: `GET /api/v1/settings/notifications/stats?kind=reachability|ups&hours=N` (`hours` до 8760; старое `days` поддерживается).

## Если уведомления не приходят

1. Канал включён? (`telegram_enabled` / `email_enabled` / `webhook_enabled`).
2. В фильтре типов есть нужный тип? Пустой список = все, но `,UPS_ON_BATTERY,` с опечаткой — нет.
3. Важность проходит фильтр (например, для `UPS_MAINS_OK` это `info`).
4. Событие вообще возникло? Смотрите «Журнал» в UI по типу.
5. Для Telegram: бот добавлен в чат, `chat_id` — именно ID чата (не username), бот не заблокирован.
6. Для email: SMTP-порт не блокируется провайдером, «Проверить email» отдаёт ошибку — смотрите её текст.
7. Журнал службы: `journalctl -u NetLynx -n 200 | grep -iE "telegram|smtp|email|webhook"`.

## Мгновенные события с оборудования (traps)

Для коммутаторов и ИБП можно принимать SNMP-трапы — тогда часть событий приходит не по опросу, а сразу (см. [UPS.md](UPS.md#трапы-от-карт-ибп) и `GET/PATCH /api/v1/settings/snmp-traps`). Приёмник: UDP **9162**, проброс стандартного порта 162 → 9162 описан в Runbook.
